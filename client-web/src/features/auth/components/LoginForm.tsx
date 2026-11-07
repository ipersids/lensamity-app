import { type SubmitEventHandler, useState } from "react";
import { useNavigate } from "react-router";
import { useLoading, useLogin } from "../../../stores/auth";
import PasswordField from "./PasswordField";
import TextField from "./TextField";

const LoginForm = () => {
  const login = useLogin();
  const isLoading = useLoading();
  const navigate = useNavigate();

  const [username, setUsername] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  const handleSubmit: SubmitEventHandler<HTMLFormElement> = async (event) => {
    event.preventDefault();

    const trimmedUsername = username.trim();

    if (!trimmedUsername || !password) {
      setError("Username and password are required.");
      return;
    }

    try {
      await login({ username: trimmedUsername, password });
      navigate("/profile", { replace: true });
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Oops, something went wrong.");
    }
  };

  return (
    <form id="login" name="login" className="auth-form" onSubmit={handleSubmit}>
      <TextField
        label={"Username"}
        id="username"
        name="username"
        autoComplete="username"
        type="text"
        value={username}
        onChange={(event) => setUsername(event.target.value)}
        disabled={isLoading}
        required
      />

      <PasswordField
        id="current-password"
        autoComplete="current-password"
        name="current-password"
        onChange={(event) => setPassword(event.target.value)}
        value={password}
        disabled={isLoading}
        label="Password"
      />

      {error && <p className="auth-error">{error}</p>}

      <button disabled={isLoading} type="submit">
        {"Log in"}
      </button>
    </form>
  );
};

export default LoginForm;
