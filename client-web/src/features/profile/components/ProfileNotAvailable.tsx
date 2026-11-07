import { UserIcon } from "@heroicons/react/24/outline";
import { NavLink } from "react-router";
import { useUser } from "../../../stores/auth";

const ProfileNotAvailable = () => {
  const user = useUser();

  return (
    <div className="profile-unavailable">
      <span className="profile-unavailable-icon" aria-hidden="true">
        <UserIcon />
      </span>
      <h1>Profile isn't available</h1>
      <p>The link may be broken, or the profile may have been removed.</p>
      {!user ? (
        <NavLink to="/signup">Join the community to explore</NavLink>
      ) : (
        <NavLink to="/profile">Go to your profile</NavLink>
      )}
    </div>
  );
};

export default ProfileNotAvailable;
