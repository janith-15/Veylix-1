package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestEveryMenuVerbIsDispatched is the test this file exists for.
//
// A menu entry that runs nothing is a lie to the operator: they pick "16)
// Certificate", and get "unknown command". The dispatcher's verb list is the only
// authority, so it is extracted from run() at test time and every menu item is
// asserted against it.
func TestEveryMenuVerbIsDispatched(t *testing.T) {
	for _, sec := range controlMenu {
		for _, it := range sec.items {
			if len(it.args) == 0 {
				t.Errorf("menu item %q (%s) has no args - it would run nothing", it.key, it.label)
				continue
			}
			verb := it.args[0]
			if _, ok := dispatchTable[verb]; !ok {
				t.Errorf("menu item %q (%s) runs %q, which run() does not dispatch -\n"+
					"  an operator selecting it would get \"unknown command %q\"",
					it.key, it.label, verb, verb)
			}
		}
	}
}

// TestEveryDispatchedVerbIsDocumented is the other direction: a command the
// operator can type but printUsage never mentions is a command nobody finds.
func TestEveryDispatchedVerbIsDocumented(t *testing.T) {
	var buf bytes.Buffer
	printUsageTo(&buf)
	usage := buf.String()

	for verb := range dispatchTable {
		if !strings.Contains(usage, verb) {
			t.Errorf("`vpn-platformctl %s` is dispatched but missing from the usage text", verb)
		}
	}
}

// TestMenuKeysAreUniqueAndContiguous catches the two mistakes that make a menu
// unusable: two items on one number (the later one is unreachable) and a gap that
// makes the operator wonder what they missed.
func TestMenuKeysAreUniqueAndContiguous(t *testing.T) {
	seen := map[string]string{}
	want := 1
	for _, sec := range controlMenu {
		for _, it := range sec.items {
			if prev, dup := seen[it.key]; dup {
				t.Errorf("key %q is used by both %q and %q", it.key, prev, it.label)
			}
			seen[it.key] = it.label

			if it.key != itoa(want) {
				t.Errorf("expected the next key to be %d, found %q (%s) - "+
					"keys must run 1..n with no gaps", want, it.key, it.label)
			}
			want++
		}
	}
	if want < 2 {
		t.Fatal("the menu has fewer than two items")
	}
}

// TestRenderMenuShowsEveryItem proves the rendering does not silently drop one.
func TestRenderMenuShowsEveryItem(t *testing.T) {
	var buf bytes.Buffer
	renderMenu(&buf, "VEYLIX CONTROL MENU")
	got := buf.String()

	if !strings.Contains(got, "VEYLIX CONTROL MENU") {
		t.Error("the header is missing")
	}
	if !strings.Contains(got, "0) Exit") {
		t.Error("the exit option is missing")
	}
	for _, sec := range controlMenu {
		if !strings.Contains(got, sec.title) {
			t.Errorf("the section %q is missing from the rendering", sec.title)
		}
		for _, it := range sec.items {
			if !strings.Contains(got, it.label) {
				t.Errorf("menu item %q (%s) is missing from the rendering", it.key, it.label)
			}
			if !strings.Contains(got, it.hint) {
				t.Errorf("the hint for %q is missing from the rendering", it.label)
			}
		}
	}
}

// itoa avoids importing strconv for one call in a test helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
