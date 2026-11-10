package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// cmdMenu is the `menu` subcommand: open the control menu explicitly.
//
// It exists because `vpn-platformctl` with no arguments already opens the menu,
// but a launcher script, a systemd ExecStart and a human all need a form that
// cannot be confused with "run the default command". It also makes the menu
// reachable when stdin is not a terminal while a controlling terminal exists -
// the same /dev/tty distinction the installer had to learn (D-124).
func cmdMenu(_ paths, args []string) error {
	fs := newFlagSet("menu")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("menu takes no arguments")
	}
	return showMenu("VEYLIX CONTROL MENU")
}

// menuItem is one line of the control menu.
//
// The menu is DATA, not a switch inside a print loop, for one reason: a test can
// then assert that every item points at a command that actually exists. A menu
// entry that runs nothing is a lie to the operator, and it is the failure mode
// this file exists to make impossible.
type menuItem struct {
	key   string   // the number typed to choose it
	label string   // what the operator reads
	hint  string   // what it does
	args  []string // passed to run() exactly as if typed on the command line
	// needsTTY marks an item that prompts for input. It is refused when the
	// menu is not attached to a terminal, because the prompt would hang.
	needsTTY bool
}

// menuSection groups items under a heading.
type menuSection struct {
	title string
	items []menuItem
}

// controlMenu is the whole thing. Keep it in sync with printUsage: both describe
// the same commands, and menu_test.go asserts every item's verb is one the
// dispatcher accepts.
var controlMenu = []menuSection{
	{
		title: "SERVICE",
		items: []menuItem{
			{key: "1", label: "Start", hint: "start veylixd", args: []string{"start"}},
			{key: "2", label: "Stop", hint: "stop veylixd", args: []string{"stop"}},
			{key: "3", label: "Restart", hint: "restart veylixd", args: []string{"restart"}},
			{key: "4", label: "Status", hint: "service, readiness and database", args: []string{"status"}},
			{key: "5", label: "Logs", hint: "show the end of the service log", args: []string{"logs"}},
			{key: "6", label: "Enable autostart", hint: "start on boot", args: []string{"enable"}},
			{key: "7", label: "Disable autostart", hint: "do not start on boot", args: []string{"disable"}},
		},
	},
	{
		title: "MAINTAIN",
		items: []menuItem{
			{key: "8", label: "Update", hint: "upgrade to the latest release", args: []string{"update"}},
			{key: "9", label: "Versions", hint: "list published releases", args: []string{"versions"}},
			{key: "10", label: "Downgrade", hint: "install an older release", args: []string{"downgrade"}, needsTTY: true},
			{key: "11", label: "Backup", hint: "take a backup now", args: []string{"backup"}},
			{key: "12", label: "Restore", hint: "restore from a backup", args: []string{"restore"}, needsTTY: true},
			{key: "13", label: "Doctor", hint: "diagnose this host", args: []string{"doctor"}},
			{key: "14", label: "Repair", hint: "re-render config and reload", args: []string{"repair"}},
		},
	},
	{
		title: "DOMAIN & ACCESS",
		items: []menuItem{
			{key: "15", label: "Change domain", hint: "point Veylix at another domain", args: []string{"domain-set"}, needsTTY: true},
			{key: "16", label: "Change ports", hint: "move HTTPS / HTTP / app port", args: []string{"ports", "set"}, needsTTY: true},
			{key: "17", label: "Certificate", hint: "check the TLS certificate", args: []string{"cert"}},
			{key: "18", label: "Reset admin", hint: "change the Super Admin password", args: []string{"reset-admin"}, needsTTY: true},
		},
	},
	{
		title: "DANGER",
		items: []menuItem{
			{key: "19", label: "Uninstall", hint: "remove the service, keep the data", args: []string{"uninstall"}, needsTTY: true},
			{key: "20", label: "Uninstall + purge", hint: "remove EVERYTHING including data", args: []string{"uninstall", "--purge-data"}, needsTTY: true},
		},
	},
}

// menuWidth is the column the hint text starts at.
const menuWidth = 24

// findMenuItem returns the item for a key, or nil.
func findMenuItem(key string) *menuItem {
	for _, sec := range controlMenu {
		for i := range sec.items {
			if sec.items[i].key == key {
				return &sec.items[i]
			}
		}
	}
	return nil
}

// renderMenu writes the menu to w. It is separate from showMenu so a test can
// assert the rendering without a terminal.
func renderMenu(w io.Writer, header string) {
	line := strings.Repeat("=", 62)
	fmt.Fprintln(w, line)
	fmt.Fprintf(w, "  %s\n", header)
	fmt.Fprintln(w, line)
	fmt.Fprintln(w)
	for _, sec := range controlMenu {
		fmt.Fprintf(w, "  %s\n", sec.title)
		for _, it := range sec.items {
			pad := menuWidth - len(it.label)
			if pad < 2 {
				pad = 2
			}
			fmt.Fprintf(w, "  %2s) %s%s%s\n", it.key, it.label, strings.Repeat(" ", pad), it.hint)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "   0) Exit")
	fmt.Fprintln(w)
	fmt.Fprintln(w, line)
}

// runFromMenu executes one selection.
//
// Every item delegates to run(), the same function main() uses. That is the
// point: the menu cannot do anything the command line cannot, so there is no
// second implementation to drift, and every --dry-run guarantee still applies.
func runFromMenu(item *menuItem, interactive bool, w io.Writer) error {
	if item.needsTTY && !interactive {
		fmt.Fprintf(w, "\n  %s needs a terminal to ask you a question; run `vpn-platformctl %s` directly.\n",
			item.label, strings.Join(item.args, " "))
		return nil
	}
	return run(item.args)
}

// menu is the interactive loop over stdin/stdout.
func menu(in io.Reader, outW io.Writer) error {
	scanner := bufio.NewScanner(in)

	for {
		fmt.Fprintln(outW)
		renderMenu(outW, "VEYLIX CONTROL MENU")
		fmt.Fprint(outW, "\nSelect an option [0-19]: ")

		if !scanner.Scan() {
			// EOF (Ctrl-D or a closed pipe): leave cleanly rather than loop.
			fmt.Fprintln(outW)
			return nil
		}
		choice := strings.TrimSpace(scanner.Text())

		if choice == "0" || strings.EqualFold(choice, "q") || strings.EqualFold(choice, "exit") {
			fmt.Fprintln(outW, "bye.")
			return nil
		}

		item := findMenuItem(choice)
		if item == nil {
			fmt.Fprintf(outW, "\n  %q is not an option. Type a number from the list, or 0 to exit.\n", choice)
			continue
		}

		fmt.Fprintln(outW)
		if err := runFromMenu(item, isInteractive(), outW); err != nil {
			// Report and return to the menu: one failed action must not close
			// the session and make the operator type their way back in.
			var ue usageError
			if errors.As(err, &ue) {
				fmt.Fprintf(outW, "\nvpn-platformctl: %v\n", ue.err)
			} else {
				fmt.Fprintf(outW, "\nvpn-platformctl: %v\n", err)
			}
			fmt.Fprintln(outW, "  (back at the menu)")
		}
		fmt.Fprint(outW, "\nPress Enter to return to the menu...")
		if !scanner.Scan() {
			return nil
		}
	}
}

// showMenu is what `vpn-platformctl` with no arguments runs.
//
// It refuses to start the loop when there is no terminal to read a choice from:
// a menu that cannot read would block forever under a pipe, and
// `vpn-platformctl | grep status` hanging is a worse bug than printing help.
func showMenu(header string) error {
	if !isInteractive() {
		printUsage(os.Stdout)
		return nil
	}
	return menu(os.Stdin, os.Stdout)
}

// isInteractive reports whether stdin is a terminal. stdout is deliberately not
// checked: `vpn-platformctl > file` at a keyboard should still work.
//
// It is a variable so tests can force each branch without a pty; production
// never assigns to it.
var isInteractive = stdinIsTTY
