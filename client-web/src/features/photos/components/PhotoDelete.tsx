import { TrashIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { Dialog } from "radix-ui";
import { useState } from "react";
import { useNavigate } from "react-router";
import useDeletePhoto from "../../../hooks/useDeletePhoto";
import { getApiError } from "../../../services/api";

type PhotoDeleteParams = {
  username: string;
  currentPhotoID: string;
  previousPhotoID?: string;
  nextPhotoID?: string;
};

const PhotoDelete = ({
  username,
  currentPhotoID,
  previousPhotoID,
  nextPhotoID,
}: PhotoDeleteParams) => {
  const [open, setOpen] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const deletePhoto = useDeletePhoto();
  const navigate = useNavigate();

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);

    if (!open) {
      setError("");
    }
  };

  const handleDeletePhoto = (e: React.MouseEvent<HTMLButtonElement, MouseEvent>) => {
    e.stopPropagation();
    e.preventDefault();
    if (deletePhoto.isPending) return;

    deletePhoto.mutate(currentPhotoID, {
      onSuccess: () => {
        if (previousPhotoID) {
          return navigate(`/users/${username}/photos/${previousPhotoID}`);
        }

        if (nextPhotoID) {
          return navigate(`/users/${username}/photos/${nextPhotoID}`);
        }

        return navigate(`/users/${username}`);
      },

      onError: (error) => {
        const err = getApiError(error);
        setError(`Error: ${err.error.message} Contact us if the error occurs again.`);
      },
    });
  };

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <Dialog.Trigger asChild>
        <button aria-label="Delete photo" className="photo-info-delete" type="button">
          <TrashIcon aria-hidden="true" />
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content">
          <Dialog.Title className="dialog-title">Delete this photo?</Dialog.Title>
          <Dialog.Description className="dialog-description">
            You won't be able to restore it later. Please be certain.
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
                onClick={(e) => handleDeletePhoto(e)}
                disabled={deletePhoto.isPending || error !== ""}
              >
                {deletePhoto.isPending && <span className="button-spinner" aria-hidden="true" />}
                {deletePhoto.isPending ? "Deleting..." : "Yes, delete photo"}
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

export default PhotoDelete;
