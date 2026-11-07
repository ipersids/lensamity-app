import type { AxiosInstance } from "axios";
import axios from "axios";

const baseURL = import.meta.env.VITE_BASE_API_URL ?? "";

// Default axios settings

axios.defaults.maxContentLength = 10 * 1024 * 1024;
axios.defaults.maxBodyLength = 10 * 1024 * 1024;
axios.defaults.withCredentials = true;
axios.defaults.redact = ["authorization", "password"];

// Auth axios instance to handle auth relaited requests

export const authApi: AxiosInstance = axios.create({
  baseURL,
  timeout: 2000,
  withCredentials: true,
  headers: {
    "Content-Type": "application/json",
  },
  formDataHeaderPolicy: "content-only",
});

authApi.interceptors.request.use((config) => {
  config.headers["Request-ID"] = crypto.randomUUID() || Date.now().toString(36);
  return config;
});

// Protected axios instance for cookie-authenticated requests.

export const internalApi: AxiosInstance = axios.create({
  baseURL,
  timeout: 5000,
  withCredentials: true,
  headers: {
    "Content-Type": "application/json",
  },
});

internalApi.interceptors.request.use((config) => {
  config.headers["Request-ID"] = crypto.randomUUID() || Date.now().toString(36);
  return config;
});

// Helper function for extracting error message

export type ApiError = {
  status?: number;
  error: {
    code: string;
    message: string;
  };
};

const isApiError = (value: unknown): value is ApiError => {
  if (!value || typeof value !== "object" || !("error" in value)) return false;

  const { error } = value;
  return (
    !!error &&
    typeof error === "object" &&
    "code" in error &&
    typeof error.code === "string" &&
    "message" in error &&
    typeof error.message === "string"
  );
};

export const getApiError = (error: unknown): ApiError => {
  if (axios.isAxiosError(error)) {
    if (!error.response) {
      return {
        error: {
          code: "network_error",
          message: "Unable to reach the server. Try again.",
        },
      };
    }

    const { data, status } = error.response;

    if (isApiError(data)) {
      if (status === 400) {
        return {
          status,
          error: {
            code: data.error.code,
            message: data.error.message,
          },
        };
      }

      if (status === 401) {
        return {
          status,
          error: {
            code: "unauthorized",
            message: "Your session has expired. Log in again to continue.",
          },
        };
      }

      return { status, ...data };
    }

    return {
      status,
      error: {
        code: `http_${status}`,
        message: "Something went wrong. Try again.",
      },
    };
  }

  return {
    error: {
      code: "unknown_error",
      message: error instanceof Error ? error.message : "Something went wrong.",
    },
  };
};
