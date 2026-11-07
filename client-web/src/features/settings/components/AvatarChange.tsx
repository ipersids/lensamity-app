import { PhotoIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { Dialog } from "radix-ui";
import { useRef, useState } from "react";
import type { Area } from "react-easy-crop";
import Cropper from "react-easy-crop";
import useUpdateAvatar from "../../../hooks/useUpdateAvatar";

const outputSize = 512;
const initialCrop = { x: 0, y: 0 };
const acceptedImageTypes = new Set(["image/jpeg", "image/png", "image/webp"]);
const maxImageBytes = 1 * 1024 * 1024;
const maxImageDimension = 4_096;
const maxImagePixels = 16_000_000;
const maxImageAspectRatio = 3;

const loadImage = (url: string) =>
  new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = reject;
    image.src = url;
  });

const createValidatedImageURL = async (file: File) => {
  if (!acceptedImageTypes.has(file.type)) {
    throw new Error("Choose a JPEG, PNG, or WebP image.");
  }

  if (file.size > maxImageBytes) {
    throw new Error("Choose an image of 1 MiB or smaller.");
  }

  const imageURL = URL.createObjectURL(file);

  try {
    const image = await loadImage(imageURL);
    const pixels = image.naturalWidth * image.naturalHeight;

    if (image.naturalWidth > maxImageDimension || image.naturalHeight > maxImageDimension) {
      throw new Error("Choose an image no larger than 4,096 pixels on either side.");
    }

    if (pixels > maxImagePixels) {
      throw new Error("Choose an image with no more than 16 megapixels.");
    }

    if (
      image.naturalWidth > image.naturalHeight * maxImageAspectRatio ||
      image.naturalHeight > image.naturalWidth * maxImageAspectRatio
    ) {
      throw new Error("Choose an image with an aspect ratio between 1:3 and 3:1.");
    }

    return imageURL;
  } catch (error) {
    URL.revokeObjectURL(imageURL);
    throw error;
  }
};

const createCroppedAvatar = async (imageURL: string, crop: Area) => {
  const image = await loadImage(imageURL);
  const canvas = document.createElement("canvas");
  canvas.width = outputSize;
  canvas.height = outputSize;

  const context = canvas.getContext("2d");
  if (!context) throw new Error("Your browser cannot crop this image.");

  context.drawImage(image, crop.x, crop.y, crop.width, crop.height, 0, 0, outputSize, outputSize);

  const blob = await new Promise<Blob | null>((resolve) =>
    canvas.toBlob(resolve, "image/webp", 0.9),
  );
  if (!blob) throw new Error("Unable to save the cropped image.");

  return new File([blob], "avatar.webp", { type: "image/webp" });
};

const AvatarChange = () => {
  const [open, setOpen] = useState(false);
  const [imageURL, setImageURL] = useState<string>();
  const [crop, setCrop] = useState(initialCrop);
  const [zoom, setZoom] = useState(1);
  const [croppedArea, setCroppedArea] = useState<Area>();
  const [error, setError] = useState("");
  const fileInput = useRef<HTMLInputElement>(null);
  const updateAvatar = useUpdateAvatar();
  const isSaving = updateAvatar.isPending;

  const resetCrop = () => {
    setCrop(initialCrop);
    setZoom(1);
    setCroppedArea(undefined);
  };

  const resetEditor = () => {
    if (imageURL) URL.revokeObjectURL(imageURL);
    setImageURL(undefined);
    resetCrop();
    setError("");
  };

  const handleOpenChange = (isOpen: boolean) => {
    if (isSaving) return;
    setOpen(isOpen);
    if (!isOpen) resetEditor();
  };

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;

    try {
      const nextImageURL = await createValidatedImageURL(file);

      if (imageURL) URL.revokeObjectURL(imageURL);
      setImageURL(nextImageURL);
      resetCrop();
      setError("");
    } catch (error) {
      setError(error instanceof Error ? error.message : "Unable to load this image.");
    }
  };

  const handleSave = async () => {
    if (!imageURL || !croppedArea || isSaving) return;

    setError("");
    try {
      const croppedFile = await createCroppedAvatar(imageURL, croppedArea);
      await updateAvatar.mutateAsync({
        avatarInfo: { contentType: croppedFile.type, size: croppedFile.size },
        file: croppedFile,
      });
      setOpen(false);
      resetEditor();
    } catch (error) {
      setError(error instanceof Error ? error.message : "Unable to save the cropped image.");
    }
  };

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <Dialog.Trigger asChild>
        <button className="settings-avatar-button settings-avatar-change-button" type="button">
          Change
        </button>
      </Dialog.Trigger>

      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content avatar-change-dialog">
          <Dialog.Title className="dialog-title">Choose a new profile photo</Dialog.Title>
          <Dialog.Description className="dialog-description">
            Position and zoom the image inside the circular frame.
          </Dialog.Description>
          <div className="avatar-change-cropper">
            {imageURL ? (
              <Cropper
                image={imageURL}
                crop={crop}
                zoom={zoom}
                aspect={1}
                objectFit="cover"
                cropShape="round"
                showGrid={false}
                onCropChange={setCrop}
                onCropComplete={(_croppedArea, croppedAreaPixels) =>
                  setCroppedArea(croppedAreaPixels)
                }
                onZoomChange={setZoom}
              />
            ) : (
              <div className="avatar-change-empty">
                <PhotoIcon aria-hidden="true" />
                <span>Choose a photo to crop.</span>
                <button
                  className="dialog-trigger avatar-change-file-button avatar-change-file-button--empty"
                  type="button"
                  onClick={() => fileInput.current?.click()}
                  disabled={isSaving}
                >
                  Choose photo
                </button>
              </div>
            )}
          </div>
          <input
            ref={fileInput}
            className="avatar-change-file-input"
            type="file"
            accept="image/jpeg,image/png,image/webp"
            onChange={handleFileChange}
          />
          {imageURL && (
            <button
              className="dialog-cancel-button avatar-change-file-button"
              type="button"
              onClick={() => fileInput.current?.click()}
              disabled={isSaving}
            >
              Choose another photo
            </button>
          )}
          <label className="avatar-change-zoom" htmlFor="avatar-zoom">
            Zoom
            <input
              id="avatar-zoom"
              type="range"
              min="1"
              max="3"
              step="0.01"
              value={zoom}
              onChange={(event) => setZoom(Number(event.target.value))}
              disabled={isSaving || !imageURL}
            />
          </label>

          <p
            className="dialog-field-note dialog-field-note-error"
            aria-live="polite"
            hidden={!error}
          >
            {error}
          </p>
          <div className="dialog-actions">
            <Dialog.Close asChild>
              <button className="dialog-cancel-button" type="button" disabled={isSaving}>
                Cancel
              </button>
            </Dialog.Close>
            <button
              className="dialog-save-button"
              type="button"
              onClick={handleSave}
              disabled={!imageURL || !croppedArea || isSaving}
            >
              {isSaving ? "Saving..." : "Save cropped photo"}
            </button>
          </div>
          <Dialog.Close asChild>
            <button
              className="dialog-close-button"
              type="button"
              aria-label="Close"
              disabled={isSaving}
            >
              <XMarkIcon />
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
};

export default AvatarChange;
