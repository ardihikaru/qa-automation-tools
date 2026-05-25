import { useState } from 'react';
import { LogOut } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { roleBadgeClass } from '../auth/api';

// UserChip is the persistent identity surface in the layout. It satisfies
// Recognition-over-Recall — the team can always see who's signed in and as
// what role without opening a menu.
//
// The logout button uses a themed confirm dialog (not native confirm()) so
// the experience stays consistent with the rest of the dashboard.
export default function UserChip() {
  const { user, logout } = useAuth();
  const [confirming, setConfirming] = useState(false);

  if (!user) return null;

  const initials = (user.fullName || user.username)
    .split(/\s+/)
    .filter(Boolean)
    .map((p) => p[0]?.toUpperCase())
    .slice(0, 2)
    .join('');

  return (
    <>
      <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-white border border-gray-200 shadow-sm">
        <span className="w-7 h-7 rounded-full bg-primary-600 text-white text-xs font-semibold flex items-center justify-center">
          {initials || '?'}
        </span>
        <div className="flex flex-col leading-tight">
          <span className="text-sm font-medium text-gray-900">{user.fullName || user.username}</span>
          <span
            className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ring-1 ring-inset w-fit mt-0.5 ${roleBadgeClass(
              user.role,
            )}`}
          >
            {user.role}
          </span>
        </div>
        <button
          type="button"
          onClick={() => setConfirming(true)}
          aria-label="Sign out"
          className="ml-1 p-1.5 rounded-full text-gray-500 hover:text-gray-900 hover:bg-gray-100 transition-colors"
        >
          <LogOut className="w-4 h-4" />
        </button>
      </div>

      {confirming && (
        <ConfirmLogout
          onCancel={() => setConfirming(false)}
          onConfirm={async () => {
            await logout();
            setConfirming(false);
          }}
        />
      )}
    </>
  );
}

function ConfirmLogout({ onCancel, onConfirm }: { onCancel: () => void; onConfirm: () => void }) {
  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40 px-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-sm p-6">
        <h2 className="text-lg font-semibold text-gray-900">Sign out?</h2>
        <p className="text-sm text-gray-500 mt-2">You can sign back in any time.</p>
        <div className="flex justify-end gap-2 mt-5">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 h-9 rounded-lg text-sm font-medium text-gray-700 hover:bg-gray-100"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="px-4 h-9 rounded-lg text-sm font-medium bg-primary-600 text-white hover:bg-primary-700"
          >
            Sign out
          </button>
        </div>
      </div>
    </div>
  );
}
