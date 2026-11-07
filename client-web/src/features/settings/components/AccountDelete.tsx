import { XMarkIcon } from "@heroicons/react/24/outline";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Dialog } from "radix-ui";
import { useState } from "react";
import { useNavigate } from "react-router";
import { getApiError } from "../../../services/api";
import usersService from "../../../services/users";
import { useAuthStore } from "../../../stores/auth";

const AccountDelete = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const clearSession = useAuthStore((state) => state.actions.clearSession);
  const [open, setOpen] = useState<boolean>(false);
  const [error, setError] = useState<string>("");

  const deleteAccount = useMutation({
    mutationFn: usersService.deleteProfile,
    onSuccess: () => {
      setOpen(false);
      queryClient.clear();
      clearSession();
      navigate(`/signup`, { replace: true });
    },
    onError: (error) => {
      const err = getApiError(error);
      setError(`Error: ${err.error.message} Contact us if the error occurs again.`);
    },
  });

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);

    if (!isOpen) {
      setError("");
    }
  };

  const handleDeleteAccount = (e: React.MouseEvent<HTMLButtonElement, MouseEvent>) => {
    e.preventDefault();
    e.stopPropagation();
    deleteAccount.mutate();
  };

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <div className="settings-account-action settings-danger-zone">
        <div>
          <h3>Delete account</h3>
          <p>Permanently remove your profile and photos.</p>
        </div>
        <Dialog.Trigger asChild>
          <button className="dialog-trigger" type="button">
            Delete account
          </button>
        </Dialog.Trigger>
      </div>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content">
          <Dialog.Title className="dialog-title">Are you sure you want to do this?</Dialog.Title>
          <Dialog.Description className="dialog-description">
            Once you delete your account, there is no going back. Please be certain.
          </Dialog.Description>
          <p
            className={`dialog-field-note dialog-field-note-error`}
            id="error-delete-profile"
            aria-live="polite"
            hidden={error === ""}
          >
            {error}
          </p>
          <div className="dialog-actions">
            <Dialog.Close asChild>
              <button className="dialog-cancel-button" type="button">
                Cancel
              </button>
            </Dialog.Close>
            <Dialog.Close asChild>
              <button
                className="dialog-save-button dialog-delete-button"
                type="button"
                onClick={(e) => handleDeleteAccount(e)}
                disabled={deleteAccount.isPending || error !== ""}
              >
                {deleteAccount.isPending && <span className="button-spinner" aria-hidden="true" />}
                {deleteAccount.isPending ? "Deleting..." : "Yes, delete my account"}
              </button>
            </Dialog.Close>
          </div>
          <Dialog.Close asChild>
            <button className="dialog-close-button" type="button" aria-label="Close">
              <XMarkIcon />
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
};

export default AccountDelete;
