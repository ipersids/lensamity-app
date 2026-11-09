// AI-generated home page content.
import { ArrowRightIcon, CameraIcon, HeartIcon, UserCircleIcon } from "@heroicons/react/24/outline";
import { Link, Navigate } from "react-router";
import { useUser } from "../stores/auth";

const HomePage = () => {
  const user = useUser();

  if (user) {
    return <Navigate to={`/users/${user.username}`} replace />;
  }

  return (
    <section className="home-page">
      <div className="home-hero">
        <div className="home-hero-copy">
          <p className="home-eyebrow">A calmer home for your photos</p>
          <h1>Your week, in focus.</h1>
          <p className="home-intro">
            Lensamity gives everyday moments a place to land. Add a photo from the last week, write
            a few words, and keep a small record of what mattered.
          </p>
          <div className="home-actions">
            <Link className="home-primary-action" to={"/signup"}>
              {"Create your profile"}
              <ArrowRightIcon className="home-action-icon" aria-hidden="true" />
            </Link>
            <Link className="home-secondary-action" to={"/login"}>
              {"Log in"}
            </Link>
          </div>
        </div>

        <div className="home-photo-composition" aria-hidden="true">
          <div className="home-photo home-photo--sunset" />
          <div className="home-photo home-photo--sky" />
          <div className="home-photo home-photo--leaf" />
          <div className="home-photo-note">
            <HeartIcon className="home-photo-note-icon" aria-hidden="true" />
            <span>Small moments count.</span>
          </div>
        </div>
      </div>

      <section className="home-story" aria-labelledby="home-story-title">
        <div>
          <p className="home-eyebrow">How it works</p>
          <h2 id="home-story-title">A simple habit for remembering more.</h2>
        </div>
        <ol className="home-steps">
          <li>
            <CameraIcon className="home-step-icon" aria-hidden="true" />
            <div>
              <h3>Add the moment</h3>
              <p>Upload a recent photo while the details are still fresh.</p>
            </div>
          </li>
          <li>
            <HeartIcon className="home-step-icon" aria-hidden="true" />
            <div>
              <h3>Give it context</h3>
              <p>Add a title and a few words that make the memory yours.</p>
            </div>
          </li>
          <li>
            <UserCircleIcon className="home-step-icon" aria-hidden="true" />
            <div>
              <h3>Keep it in the community</h3>
              <p>Share with the Lensamity community or keep your profile private.</p>
            </div>
          </li>
        </ol>
      </section>

      <section className="home-closing">
        <div>
          <p className="home-eyebrow">Start with one photo</p>
          <h2>A lighter way to look back.</h2>
        </div>
        <Link className="home-primary-action" to={"/signup"}>
          {"Create your profile"}
          <ArrowRightIcon className="home-action-icon" aria-hidden="true" />
        </Link>
      </section>
    </section>
  );
};

export default HomePage;
