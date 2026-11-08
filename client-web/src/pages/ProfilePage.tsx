import { useParams } from "react-router";
import {
  ProfileHeader,
  ProfileNotAvailable,
  ProfilePhotos,
  ProfileShell,
  useProfile,
} from "../features/profile";

const ProfilePage = () => {
  const { username } = useParams<{ username: string }>();
  const { isPending, isError, data: user } = useProfile(username);

  if (!username || isError) {
    return <ProfileNotAvailable />;
  }

  if (isPending || !user) {
    return null;
  }

  return (
    <ProfileShell>
      <ProfileHeader
        profile={{
          username: user.username,
          displayName: user.displayName,
          photoCount: user.photoCount,
          about: user.about,
          avatarURL: user.avatar?.url,
          canEdit: user.canEdit,
          canViewPhotos: user.canViewPhotos,
        }}
      />

      <ProfilePhotos
        username={username}
        canViewPhotos={user.canViewPhotos}
        canEdit={user.canEdit}
      />
    </ProfileShell>
  );
};

export default ProfilePage;
