import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Card, PageHeader, Button, Badge, Dialog, Input, useToast } from '../components/ui'
import { KeyRound, Palette, LogOut, Mail, ShieldCheck, User, Phone } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'
import { useProfileStore } from '../stores/profileStore'
import { api } from '../lib/api'
import { MIN_PASSWORD_LENGTH, passwordViolations } from '../lib/passwordPolicy'
import './profile.css'

/**
 * Profile (D-041) — "email හැර අනෙක් සියල්ල".
 *
 * Every field here is the account's OWN, and the API acts on the session's user id,
 * never on an id from the browser. The e-mail address is shown but NOT editable:
 * it is the login identifier and the recovery target, so a change is a code sent to
 * the NEW address (Security tab).
 */
/** Today in ISO form, used as the date input's upper bound. Computed once: a
 *  `new Date()` inside render makes the component impure for the compiler. */
const TODAY_ISO = new Date().toISOString().slice(0, 10)

/** Lower bound for the picker: 120 years back. The server enforces 13+, so the
 *  calendar simply opens on a sane range instead of the year 0001. */
const MIN_BIRTHDAY_ISO = new Date(
  Date.now() - 120 * 365.25 * 24 * 60 * 60 * 1000,
).toISOString().slice(0, 10)

export function ProfilePage() {
  const user = useAuthStore((s) => s.user)
  const signOut = useAuthStore((s) => s.signOut)
  const signOutOthers = useAuthStore((s) => s.signOutOthers)
  const changePassword = useAuthStore((s) => s.changePassword)
  const pending = useAuthStore((s) => s.pending)
  const navigate = useNavigate()
  const toast = useToast()

  const profile = useProfileStore((s) => s.profile)
  const saving = useProfileStore((s) => s.saving)
  const error = useProfileStore((s) => s.error)
  const emailChange = useProfileStore((s) => s.emailChange)
  const load = useProfileStore((s) => s.load)
  const save = useProfileStore((s) => s.save)
  const usernameFree = useProfileStore((s) => s.usernameFree)
  const requestEmailChange = useProfileStore((s) => s.requestEmailChange)
  const confirmEmailChange = useProfileStore((s) => s.confirmEmailChange)
  const cancelEmailChange = useProfileStore((s) => s.cancelEmailChange)

  const [section, setSection] = useState<'account' | 'contact' | 'security'>('account')
  // D3: "sign out other devices". Kept local to the page: it is a one-shot
  // action with a result to report, not state anything else renders.
  const [endingOthers, setEndingOthers] = useState(false)

  // Seed the drafts from the server copy ONCE IT ARRIVES.
  //
  // A `useState` initialiser reads its value only on the first render, so this
  // page used to lock the drafts to '' whenever `profile` was still loading at
  // mount — which is every time, because `load()` runs in an effect. The header
  // showed "Kasun Perera" from the auth store while "Full name" stayed blank
  // (I-39). The guard at render time is on `user`, not `profile`, so the form
  // really does paint before the profile exists.
  //
  // The effect below seeds the drafts the first time a profile arrives and then
  // stops touching them, so it cannot overwrite what the person is typing.
  const [displayName, setDisplayName] = useState(() => profile?.display_name ?? '')
  const [username, setUsername] = useState(() => profile?.username ?? '')
  const [birthday, setBirthday] = useState(() => profile?.birthday ?? '')
  const [bio, setBio] = useState(() => profile?.bio ?? '')
  // Kept for the avatar preview, but no longer editable: asking a person to paste
  // an image URL was a field nobody could use.
  const [avatarUrl, setAvatarUrl] = useState(() => profile?.avatar_url ?? '')
  const [usernameHint, setUsernameHint] = useState('')

  // Contact draft.
  const [phone, setPhone] = useState(() => profile?.phone ?? '')
  const [whatsapp, setWhatsapp] = useState(() => profile?.whatsapp ?? '')
  const [telegram, setTelegram] = useState(() => profile?.telegram ?? '')
  const [discord, setDiscord] = useState(() => profile?.discord ?? '')

  // Password dialog.
  const [pwOpen, setPwOpen] = useState(false)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwError, setPwError] = useState('')

  // E-mail change.
  const [newEmail, setNewEmail] = useState('')
  const [emailPassword, setEmailPassword] = useState('')
  const [code, setCode] = useState('')

  // Verification link request (the "verify e-mail" button next to the badge).
  const [verifyBusy, setVerifyBusy] = useState(false)

  useEffect(() => {
    void load()
  }, [load])

  /*
   * Seed the drafts the first time the server copy arrives (I-39).
   *
   * A `useState` initialiser runs once, on the first render; at that moment
   * `profile` is still null because `load()` is in flight, so every draft locked
   * to ''. The page therefore showed the person's name in the header (from the
   * auth store) and an EMPTY "Full name" field.
   *
   * `seededRef` makes this a one-shot: once the drafts have been filled from a
   * profile, later saves echo back through the store and must not clobber what
   * the person is typing.
   */
  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current || !profile) return
    seededRef.current = true
    setDisplayName(profile.display_name ?? '')
    setUsername(profile.username ?? '')
    setBirthday(profile.birthday ?? '')
    setBio(profile.bio ?? '')
    setAvatarUrl(profile.avatar_url ?? '')
    setPhone(profile.phone ?? '')
    setWhatsapp(profile.whatsapp ?? '')
    setTelegram(profile.telegram ?? '')
    setDiscord(profile.discord ?? '')
  }, [profile])

  // The drafts are seeded from the server copy. Doing it in an effect would set
  // state during render (and the lint rule rightly flags the extra render), so the
  // form is keyed on the profile instead: a new server copy remounts the inputs
  // with their current values, and a save simply re-renders — no sync loop.
  if (!user) return null

  /**
   * Ask for a verification link.
   *
   * The toast tells the truth about delivery: with a relay the link is in the
   * mailbox, without one an operator has to hand it over. Saying "check your
   * inbox" when nothing was sent is the exact lie D-041 forbids.
   */
  const requestVerification = async () => {
    setVerifyBusy(true)
    try {
      const res = await api.requestEmailVerification()
      if (res.delivery === 'smtp') {
        toast('success', 'Verification link sent', `Check ${res.email} and open the link`)
      } else {
        toast(
          'info',
          'No mail relay configured',
          res.link
            ? 'Open the link shown on the server console to verify'
            : 'Ask an operator to hand you the verification link',
        )
      }
    } catch (err) {
      toast('error', 'Could not send the link', err instanceof Error ? err.message : 'Try again')
    } finally {
      setVerifyBusy(false)
    }
  }

  const resetPwForm = () => {
    setCurrent('')
    setNext('')
    setConfirmPw('')
    setPwError('')
  }

  const submitPassword = async (e: FormEvent) => {
    e.preventDefault()
    const violations = passwordViolations(next)
    if (violations.length > 0) {
      setPwError(`New password ${violations[0]}.`)
      return
    }
    if (next !== confirmPw) {
      setPwError('The new passwords do not match.')
      return
    }
    // A Google-only account has no current password yet; the server accepts an
    // empty current password exactly when none is set, so the first set is
    // self-service and must not be blocked by this form.
    if (!current && profile?.has_password) {
      setPwError('Enter your current password.')
      return
    }
    setPwError('')
    const { error: reason, delivery } = await changePassword(current, next)
    if (reason) {
      setPwError(reason)
      return
    }
    setPwOpen(false)
    resetPwForm()
    // Say what actually happened: with a relay the owner gets a warning mail;
    // without one they are told, rather than left assuming a message was sent.
    toast(
      'success',
      'Password updated',
      delivery === 'smtp'
        ? 'All other sessions were signed out. We e-mailed you a confirmation.'
        : 'All other sessions were signed out. (No mail relay: no confirmation e-mail was sent.)',
    )
  }

  // D3: end every other session without having to rotate the password.
  const endOtherSessions = async () => {
    setEndingOthers(true)
    try {
      const { ended, error: reason } = await signOutOthers()
      if (reason) {
        // The server refused; say so rather than implying a device was evicted.
        toast('error', 'Other devices were not signed out', reason)
        return
      }
      toast(
        'success',
        ended && ended > 0 ? `${ended} other device${ended === 1 ? '' : 's'} signed out` : 'No other devices',
        ended && ended > 0
          ? 'Only this browser is still signed in.'
          : 'This was the only signed-in session for your account.',
      )
    } finally {
      setEndingOthers(false)
    }
  }

  const saveAccount = async (e: FormEvent) => {
    e.preventDefault()
    const patch: Parameters<typeof save>[0] = {
      display_name: displayName.trim(),
      username: username.trim(),
      bio: bio.trim(),
      birthday: birthday.trim(),
    }
    if (await save(patch)) return
    // The topbar reads the auth store, so keep the name in sync for an instant
    // visible change instead of waiting for a reload.
    useAuthStore.setState({ user: { ...user, name: displayName.trim() } })
    toast('success', 'Profile saved', 'Your account details were updated')
  }

  const saveContacts = async (e: FormEvent) => {
    e.preventDefault()
    const reason = await save({
      phone: phone.trim(),
      whatsapp: whatsapp.trim(),
      telegram: telegram.trim(),
      discord: discord.trim(),
    })
    if (!reason) toast('success', 'Contacts saved', 'We will use these to reach you')
  }

  const checkUsername = async () => {
    const u = username.trim().toLowerCase()
    if (u.length < 3) {
      setUsernameHint('At least 3 characters')
      return
    }
    setUsernameHint((await usernameFree(u)) ? 'Available ✓' : 'Already in use')
  }

  const startEmailChange = async (e: FormEvent) => {
    e.preventDefault()
    const reason = await requestEmailChange(newEmail.trim(), emailPassword)
    if (reason) return
    setEmailPassword('')
    toast('info', 'Code sent', `Confirm the code sent to ${newEmail.trim()}`)
  }

  const finishEmailChange = async (e: FormEvent) => {
    e.preventDefault()
    const reason = await confirmEmailChange(code.trim())
    if (reason) return
    setCode('')
    setNewEmail('')
    toast('success', 'E-mail changed', 'Every other session was signed out')
  }

  const avatar = avatarUrl.trim() || profile?.avatar_url || ''
  // Shown next to the date picker. A live preview is the whole point of storing a
  // date instead of an age, so it follows the saved profile rather than a number
  // typed by hand.
  const age = profile?.age && profile.age > 0 ? profile.age : 0
  const initial = (displayName || user.name || user.email).slice(0, 1).toUpperCase()

  return (
    <div className="dash" data-testid="profile-page">
      <PageHeader
        title="Profile & preferences"
        subtitle="Your account, how we reach you, and your security"
        actions={
          <Button size="sm" variant="outline" leading={<LogOut size={14} />}
            onClick={() => { void signOut(); navigate('/login') }}>
            Sign out
          </Button>
        }
      />

      {error && <p className="prof-alert" role="alert">{error}</p>}

      <Card title="Account">
        <div className="prof-row">
          <span className="app-avatar app-user__avatar" aria-hidden="true">
            {avatar ? <img src={avatar} alt="" className="prof-avatar-img" /> : initial}
          </span>
          <div className="flex flex-col gap-1">
            <span className="ui-strong">{profile?.display_name || user.name}</span>
            <span className="ui-muted text-[12.5px]">
              {profile?.username ? '@' + profile.username : user.email}
            </span>
            <span className="ui-muted text-[12px]">
              <Mail size={12} aria-hidden="true" /> {profile?.email ?? user.email}{' '}
              {profile?.email_verified
                ? <Badge tone="success">verified</Badge>
                : <>
                    <Badge tone="warning">not verified</Badge>
                    {/* Without this button the badge was a dead end: a client who
                        signed up could never confirm the address, and login
                        refuses an unverified account (auth.ErrEmailNotVerified). */}
                    <Button
                      variant="outline"
                      size="sm"
                      loading={verifyBusy}
                      onClick={requestVerification}
                    >
                      Verify e-mail
                    </Button>
                  </>}
              {profile?.google_linked && <Badge tone="info">Google linked</Badge>}
            </span>
          </div>
          <Badge tone="primary" className="prof-role">{user.role}</Badge>
        </div>
      </Card>

      <div className="prof-tabs" role="tablist" aria-label="Profile sections">
        <button type="button" role="tab" aria-selected={section === 'account'}
          className={`prof-tab ${section === 'account' ? 'is-active' : ''}`}
          onClick={() => setSection('account')}>
          <User size={14} aria-hidden="true" /> Account
        </button>
        <button type="button" role="tab" aria-selected={section === 'contact'}
          className={`prof-tab ${section === 'contact' ? 'is-active' : ''}`}
          onClick={() => setSection('contact')}>
          <Phone size={14} aria-hidden="true" /> Contact
        </button>
        <button type="button" role="tab" aria-selected={section === 'security'}
          className={`prof-tab ${section === 'security' ? 'is-active' : ''}`}
          onClick={() => setSection('security')}>
          <ShieldCheck size={14} aria-hidden="true" /> Security
        </button>
      </div>


      {section === 'account' && (
        <Card title="Your details">
          <form onSubmit={saveAccount} className="ui-col" noValidate>
            <Input label="Full name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
            <div className="prof-inline">
              <Input label="Username (used to sign in)" value={username}
                hint={usernameHint || 'Letters, digits, dot, hyphen or underscore'}
                onChange={(e) => { setUsername(e.target.value); setUsernameHint('') }}
                onBlur={() => void checkUsername()} />
              {/* A birth date instead of an age: the age is then always right,
                  including the day it changes, and nobody has to do arithmetic.
                  Wrapped in `ui-field` so the label, spacing and hint match every
                  other input on the page. */}
              <div className="ui-field">
                <label className="ui-field__label" htmlFor="birthday">Date of birth</label>
                <input
                  id="birthday"
                  type="date"
                  className="ui-input"
                  value={birthday}
                  min={MIN_BIRTHDAY_ISO}
                  max={TODAY_ISO}
                  onChange={(e) => setBirthday(e.target.value)}
                />
                <span className="ui-field__hint">
                  {age > 0 ? `${age} years old` : 'Pick the date — we work out your age'}
                </span>
              </div>
            </div>
            <label className="ui-col" style={{ gap: 6 }}>
              <span className="ui-strong text-[12.5px]">About you</span>
              <textarea className="ui-textarea" rows={3} maxLength={500} value={bio}
                onChange={(e) => setBio(e.target.value)} placeholder="Optional — visible to staff" />
            </label>
            <div className="prof-actions">
              <Button type="submit" loading={saving}>Save changes</Button>
            </div>
          </form>
          <p className="ui-muted prof-note">
            Your e-mail address cannot be changed here: it is your sign-in and recovery
            address. Change it from the <strong>Security</strong> tab, where a code is sent
            to the new address.
          </p>
        </Card>
      )}

      {section === 'contact' && (
        <Card title="How we reach you">
          <form onSubmit={saveContacts} className="ui-col" noValidate>
            <div className="prof-inline">
              <Input label="Phone" placeholder="+94 77 123 4567" value={phone}
                onChange={(e) => setPhone(e.target.value)} />
              <Input label="WhatsApp" placeholder="+94 77 123 4567" value={whatsapp}
                onChange={(e) => setWhatsapp(e.target.value)} />
            </div>
            <div className="prof-inline">
              <Input label="Telegram" placeholder="@yourname" value={telegram}
                onChange={(e) => setTelegram(e.target.value)} />
              <Input label="Discord" placeholder="name#1234" value={discord}
                onChange={(e) => setDiscord(e.target.value)} />
            </div>
            <div className="prof-actions">
              <Button type="submit" loading={saving}>Save contacts</Button>
            </div>
          </form>
        </Card>
      )}


      {section === 'security' && (
        <>
          <Card title="Password">
            {profile?.has_password ? (
              <p className="ui-muted mt-0 mb-4 text-[12.5px]">
                Changing your password signs out every other device. This browser stays
                signed in.
              </p>
            ) : (
              <p className="ui-muted mt-0 mb-4 text-[12.5px]">
                You signed in with Google and have no password yet. Set one to also sign
                in with your e-mail and password — or keep using Google. Changing it
                later signs out every other device.
              </p>
            )}
            <Button variant="outline" leading={<KeyRound size={14} />}
              onClick={() => { resetPwForm(); setPwOpen(true) }}>
              {profile?.has_password ? 'Change password' : 'Set a password'}
            </Button>
          </Card>

          <Card title="Signed-in devices">
            <p className="ui-muted mt-0 mb-4 text-[12.5px]">
              This browser stays signed in. Every other device is signed out
              immediately — use it if you left a session open somewhere you no
              longer hold.
            </p>
            <Button variant="outline" leading={<LogOut size={14} />}
              loading={endingOthers}
              onClick={endOtherSessions}>
              Sign out other devices
            </Button>
          </Card>

          <Card title="E-mail address">
            <p className="ui-muted mt-0 mb-3 text-[12.5px]">
              Current address: <strong>{profile?.email}</strong>
              {profile?.email_verified ? ' (verified)' : ' — not verified yet'}
            </p>
            {!emailChange ? (
              <form onSubmit={startEmailChange} className="ui-col" noValidate>
                <Input label="New e-mail address" type="email" value={newEmail}
                  onChange={(e) => setNewEmail(e.target.value)} />
                <Input label="Confirm with your password" type="password"
                  autoComplete="current-password" value={emailPassword}
                  hint="Asked for your password so a borrowed session cannot move your account"
                  onChange={(e) => setEmailPassword(e.target.value)} />
                <div className="prof-actions">
                  <Button type="submit" loading={saving}>Send code to the new address</Button>
                </div>
              </form>
            ) : (
              <form onSubmit={finishEmailChange} className="ui-col" noValidate>
                <p className="ui-muted mt-0 text-[12.5px]">
                  A code was sent to <strong>{emailChange.to}</strong>. It expires in{' '}
                  {emailChange.expires_minutes} minutes.
                  {emailChange.delivery === 'outbox' && (
                    <> <em>No mail relay is configured on this server</em>, so the code went to
                    the outbox instead of being delivered.</>
                  )}
                </p>
                {emailChange.code && (
                  <p className="prof-note">Local operator code: <code>{emailChange.code}</code></p>
                )}
                <Input label="6-digit code" inputMode="numeric" maxLength={6} value={code}
                  onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, ''))} />
                <div className="prof-actions">
                  <Button type="submit" loading={saving}>Confirm change</Button>
                  <Button type="button" variant="ghost" onClick={cancelEmailChange}>Cancel</Button>
                </div>
              </form>
            )}
          </Card>

          <Card title="Appearance">
            <p className="ui-muted mt-0 mb-3 text-[12.5px]">
              Theme, density and motion are personal preferences, stored in this browser.
            </p>
            <Button variant="outline" leading={<Palette size={14} />}
              onClick={() => window.dispatchEvent(new Event('veylix:open-theme-studio'))}>
              Open theme studio
            </Button>
          </Card>
        </>
      )}

      <Dialog open={pwOpen} onClose={() => setPwOpen(false)}
        title={profile?.has_password ? 'Change password' : 'Set a password'}
        footer={
          <>
            <Button variant="outline" onClick={() => setPwOpen(false)}>Cancel</Button>
            <Button type="submit" form="change-password-form" loading={pending}>
              {profile?.has_password ? 'Update password' : 'Set password'}
            </Button>
          </>
        }>
        <form id="change-password-form" onSubmit={submitPassword} className="ui-col" noValidate>
          {profile?.has_password ? (
            <Input label="Current password" type="password" autoComplete="current-password"
              value={current} onChange={(e) => setCurrent(e.target.value)} />
          ) : (
            <p className="ui-muted mt-0 mb-1 text-[12.5px]">
              You have no password yet (you signed in with Google), so nothing to
              confirm — just choose one.
            </p>
          )}
          <Input label="New password" type="password" autoComplete="new-password"
            hint={`At least ${MIN_PASSWORD_LENGTH} characters`} value={next}
            onChange={(e) => setNext(e.target.value)} />
          <Input label="Confirm new password" type="password" autoComplete="new-password"
            error={pwError || undefined} value={confirmPw}
            onChange={(e) => setConfirmPw(e.target.value)} />
        </form>
      </Dialog>
    </div>
  )
}
