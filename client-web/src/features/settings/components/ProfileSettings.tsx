import { useState } from "react";
import { Navigate } from "react-router";
import type { Visibility } from "../../../services/users";
import { useUser } from "../../../stores/auth";
import { useProfile, useUpdateProfile } from "../../profile";
import AvatarSettings from "./AvatarSettings";

type ProfileUpdateProps = {
  username: string;
  displayName: string;
  about?: string;
  visibility: Visibility;
};

const ProfileUpdate = ({ username, displayName, about, visibility }: ProfileUpdateProps) => {
  const updateProfile = useUpdateProfile();
  const [updatedDisplayName, setUpdatedDisplayName] = useState<string | undefined>(displayName);
  const [updatedAbout, setUpdatedAbout] = useState<string | undefined>(about);
  const [updatedVisibility, setUpdatedVisibility] = useState<Visibility>(visibility);

  const isUpdated =
    (updatedDisplayName !== displayName &&
      !(updatedDisplayName === undefined && displayName === username)) ||
    updatedAbout !== about ||
    updatedVisibility !== visibility;

  const handleClick = (e: React.MouseEvent<HTMLButtonElement, MouseEvent>) => {
    e.stopPropagation();
    e.preventDefault();
    updateProfile.mutate({
      displayName: updatedDisplayName === "" || !updatedDisplayName ? username : updatedDisplayName,
      about: updatedAbout,
      visibility: updatedVisibility,
    });
  };

  const handleVisibilityChange = (
    e: React.MouseEvent<HTMLButtonElement, MouseEvent>,
    value: Visibility,
  ) => {
    e.stopPropagation();
    e.preventDefault();
    setUpdatedVisibility(value);
  };

  return (
    <>
      <div className="settings-field">
        <label>
          Display name
          <input
            type="text"
            value={updatedDisplayName ?? ""}
            placeholder={username}
            aria-describedby="profile-display-name-note"
            onChange={(e) => {
              e.preventDefault();
              setUpdatedDisplayName(e.target.value === "" ? undefined : e.target.value);
            }}
          />
        </label>
        <p className="settings-field-note" id="profile-display-name-note">
          By default, your display name is the same as your username.
        </p>
      </div>

      <label>
        About
        <textarea
          value={updatedAbout}
          rows={4}
          maxLength={300}
          onChange={(e) => {
            e.preventDefault();
            setUpdatedAbout(e.target.value === "" ? undefined : e.target.value);
          }}
        />
      </label>

      <fieldset className="settings-visibility-field">
        <legend className="settings-field-title">Profile visibility</legend>
        <div className="settings-visibility-switcher" aria-describedby="profile-visibility-note">
          <button
            type="button"
            className={updatedVisibility === "public" ? "active" : undefined}
            aria-pressed={updatedVisibility === "public"}
            onClick={(e) => handleVisibilityChange(e, "public")}
          >
            Public
          </button>
          <button
            type="button"
            className={updatedVisibility === "private" ? "active" : undefined}
            aria-pressed={updatedVisibility === "private"}
            onClick={(e) => handleVisibilityChange(e, "private")}
          >
            Private
          </button>
        </div>
        <p className="settings-field-note" id="profile-visibility-note">
          {`Photos in your profile will be visible ${updatedVisibility === "public" ? "to all logged-in users." : "only to you."}`}
        </p>
      </fieldset>

      <button
        className="settings-save-button"
        type="button"
        onClick={handleClick}
        disabled={!isUpdated}
      >
        Update profile
      </button>
    </>
  );
};

const ProfileSettings = () => {
  const currentUser = useUser();
  const { isPending, isError, data: profile } = useProfile(currentUser?.username);

  if (!currentUser) {
    return <Navigate to="/login" replace />;
  }

  if (isError) {
    return <p>Error</p>;
  }

  if (isPending || !profile) {
    return null;
  }

  return (
    <form className="settings-form">
      <h2>Profile</h2>
      <p className="settings-intro">
        Information you add here is visible to anyone who can view your profile.
      </p>
      <AvatarSettings url={profile.avatar?.url} displayName={profile.displayName} />
      <ProfileUpdate
        username={profile.username}
        displayName={profile.displayName}
        about={profile.about}
        visibility={profile.visibility}
      />
    </form>
  );
};

export default ProfileSettings;
