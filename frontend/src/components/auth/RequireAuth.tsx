import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore, type UserRole } from '../../stores/authStore'

/** Role-aware landing route. */
// oxlint-disable-next-line react/only-export-components -- role routing helper is tightly coupled to the guard component
export function roleHome(role: UserRole): string {
  if (role === 'admin') return '/admin'
  if (role === 'client') return '/client'
  return '/dashboard'
}

/**
 * Route guard:
 *  1. While the session is still being restored (`status === 'unknown'`) render a
 *     small splash instead of redirecting — otherwise a reload on a protected
 *     route would bounce to /login even though the HttpOnly cookie is valid.
 *  2. No accounts exist yet → /setup (there is nobody to sign in as).
 *  3. Unauthenticated → /login (keeps the intended destination).
 *  4. Authenticated but wrong role → silently redirected to the user's own home
 *     (no toast during render — navigation itself is the feedback).
 */
export function RequireAuth({ roles }: { roles?: UserRole[] }) {
  const user = useAuthStore((s) => s.user)
  const status = useAuthStore((s) => s.status)
  const bootstrapRequired = useAuthStore((s) => s.bootstrapRequired)
  const location = useLocation()

  if (status === 'unknown') {
    return (
      <div className="auth-boot" role="status" aria-live="polite">
        <span className="auth-boot__spinner" aria-hidden="true" />
        <span className="ui-muted">Restoring your session…</span>
      </div>
    )
  }

  // No accounts exist yet → the wizard, before the login form.
  if (status === 'anon' && bootstrapRequired) {
    return <Navigate to="/setup" replace />
  }

  if (status === 'anon' || !user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  if (roles && !roles.includes(user.role)) {
    return <Navigate to={roleHome(user.role)} replace />
  }
  return <Outlet />
}
