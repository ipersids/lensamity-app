import zxcvbn from "zxcvbn";
import {
  MAX_USERNAME_LENGTH,
  MIN_PASSWORD_LENGTH,
  MIN_USERNAME_LENGTH,
  USERNAME_REGEX,
} from "../../constants";

type PaswordValidationResult = {
  score: number;
  description: string;
  feedback: string;
};

const scoreToWord = (score: number): string => {
  return ["Very Weak", "Very Weak", "Weak", "Good", "Strong"][score] ?? "Unknown";
};

export const validatePassword = (
  password: string,
  input: string[] = [],
): PaswordValidationResult => {
  const res = zxcvbn(password, input);

  if (res.score > 2 && password.length < MIN_PASSWORD_LENGTH) {
    return {
      score: 2,
      description: "Good",
      feedback: "Use at least 15 characters.",
    };
  }

  return {
    score: res.score,
    description: scoreToWord(res.score),
    feedback: res.feedback.warning || res.feedback.suggestions[0] || "",
  };
};

export const validateUsername = (username: string): string => {
  if (!username) return "";

  if (username.length < MIN_USERNAME_LENGTH || username.length > MAX_USERNAME_LENGTH) {
    return `Use ${MIN_USERNAME_LENGTH} to ${MAX_USERNAME_LENGTH} characters`;
  }

  return !USERNAME_REGEX.test(username)
    ? "Use Latin letters, numbers, hyphens, and underscores"
    : "";
};

export const normalizeText = (value: string) => value.trim().normalize("NFC");
