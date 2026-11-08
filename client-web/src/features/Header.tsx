import { Bars3Icon } from "@heroicons/react/24/outline";
import { useEffect, useRef, useState } from "react";
import { NavLink, useNavigate } from "react-router";
import { useLoading, useLogout, useUser } from "../stores/auth";

const Header = () => {
  const user = useUser();
  const logout = useLogout();
  const isLoading = useLoading();
  const navigate = useNavigate();
  const [isAccountMenuOpen, setIsAccountMenuOpen] = useState<boolean>(false);
  const accountMenuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isAccountMenuOpen) {
      return;
    }

    const handlePointerDown = (event: PointerEvent) => {
      if (!accountMenuRef.current?.contains(event.target as Node)) {
        setIsAccountMenuOpen(false);
      }
    };

    document.addEventListener("pointerdown", handlePointerDown);

    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [isAccountMenuOpen]);

  const handleLogout = async () => {
    setIsAccountMenuOpen(false);
    await logout();
    navigate("/", { replace: true });
  };

  return (
    <header className="site-header">
      <NavLink className="brand-link" to="/">
        Lensamity
      </NavLink>

      {user ? (
        <nav className="site-nav" aria-label="Account">
          <div className="account-menu" ref={accountMenuRef}>
            <button
              className="account-menu-trigger"
              type="button"
              aria-expanded={isAccountMenuOpen}
              aria-haspopup="menu"
              onClick={() => setIsAccountMenuOpen((current) => !current)}
            >
              <span className="nav-user">{user.displayName}</span>
              <Bars3Icon className="account-menu-icon" aria-hidden="true" focusable="false" />
            </button>

            {isAccountMenuOpen && (
              <div className="account-menu-panel" role="menu">
                <NavLink
                  to={`/users/${user.username}`}
                  end
                  role="menuitem"
                  onClick={() => setIsAccountMenuOpen(false)}
                >
                  Profile
                </NavLink>
                <NavLink
                  to="/upload"
                  end
                  role="menuitem"
                  onClick={() => setIsAccountMenuOpen(false)}
                >
                  Add photo
                </NavLink>
                <NavLink
                  to="/settings"
                  end
                  role="menuitem"
                  onClick={() => setIsAccountMenuOpen(false)}
                >
                  Settings
                </NavLink>
                <button disabled={isLoading} onClick={handleLogout} type="button" role="menuitem">
                  Log out
                </button>
              </div>
            )}
          </div>
        </nav>
      ) : (
        <nav className="site-nav" aria-label="Auth">
          <NavLink to="/login" end>
            Log in
          </NavLink>
          <NavLink className="nav-button" to="/signup" end>
            Sign up
          </NavLink>
        </nav>
      )}
    </header>
  );
};

export default Header;
