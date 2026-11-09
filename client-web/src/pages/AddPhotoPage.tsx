import { PhotoIcon } from "@heroicons/react/24/outline";
import { type ChangeEventHandler, type DragEventHandler, useEffect, useState } from "react";
import { useNavigate } from "react-router";
import useAddPhoto from "../hooks/useAddPhoto";
import { getApiError } from "../services/api";
import { useUser } from "../stores/auth";

const DATE_ERROR = "Choose today or one of the previous six days in UTC.";

const toDateInputValue = (date: Date) => date.toISOString().slice(0, 10);

const getDateRange = () => {
  const today = new Date();
  const weekAgo = new Date(today);
  weekAgo.setUTCDate(today.getUTCDate() - 6);

  return {
    max: toDateInputValue(today),
    min: toDateInputValue(weekAgo),
  };
};

const toApiDate = (value: string) => {
  const [year, month, day] = value.split("-");
  return `${day}-${month}-${year}`;
};

const AddPhotoPage = () => {
  const navigate = useNavigate();
  const currentUser = useUser();
  const [dateRange, setDateRange] = useState(getDateRange);
  const { min: minDate, max: maxDate } = dateRange;
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string>("");
  const [isDragging, setIsDragging] = useState<boolean>(false);
  const [photoDate, setPhotoDate] = useState<string>(maxDate);
  const dateError = !photoDate || photoDate < minDate || photoDate > maxDate ? DATE_ERROR : "";
  const [uploadError, setUploadError] = useState<string>("");
  const [title, setTitle] = useState<string>("");
  const [description, setDescription] = useState<string>("");
  const addPhoto = useAddPhoto();

  useEffect(() => {
    return () => {
      if (previewUrl) {
        URL.revokeObjectURL(previewUrl);
      }
    };
  }, [previewUrl]);

  const selectFile = (file?: File) => {
    if (file?.type.startsWith("image/")) {
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
    }
  };

  const handleFileChange: ChangeEventHandler<HTMLInputElement> = (event) => {
    selectFile(event.target.files?.[0]);
  };

  const handleTitleChange: ChangeEventHandler<HTMLInputElement> = (event) => {
    event.preventDefault();
    setTitle(event.target.value);
  };

  const handleDescriptionChange: ChangeEventHandler<HTMLTextAreaElement> = (event) => {
    event.preventDefault();
    setDescription(event.target.value);
  };

  const handleDragOver: DragEventHandler<HTMLLabelElement> = (event) => {
    event.preventDefault();
    setIsDragging(true);
  };

  const handleDrop: DragEventHandler<HTMLLabelElement> = (event) => {
    event.preventDefault();
    setIsDragging(false);
    selectFile(event.dataTransfer.files[0]);
  };

  const handleDateChange: ChangeEventHandler<HTMLInputElement> = (event) => {
    setPhotoDate(event.target.value);
  };

  const handleUpload = async () => {
    if (!selectedFile || !currentUser) return;

    setUploadError("");

    const range = getDateRange();
    setDateRange(range);
    if (!photoDate || photoDate < range.min || photoDate > range.max) {
      setUploadError(DATE_ERROR);
      return;
    }

    try {
      const photoID = await addPhoto.mutateAsync({
        photoInfo: {
          date: toApiDate(photoDate),
          contentType: selectedFile.type,
          size: selectedFile.size,
          title,
          description,
        },
        file: selectedFile,
      });

      navigate(
        `/users/${encodeURIComponent(currentUser.username)}/photos/${encodeURIComponent(photoID)}`,
      );
    } catch (error: unknown) {
      setUploadError(getApiError(error).error.message);
    }
  };

  return (
    <section className="add-photo-page">
      <form className="add-photo-form">
        <h1>Add photo</h1>

        <label
          className={`photo-upload-area${isDragging ? " dragging" : ""}`}
          onDragOver={handleDragOver}
          onDragLeave={() => setIsDragging(false)}
          onDrop={handleDrop}
        >
          {previewUrl ? (
            <img src={previewUrl} alt="" />
          ) : (
            <span className="photo-upload-icon">
              <PhotoIcon aria-hidden="true" />
            </span>
          )}

          <strong>{selectedFile?.name || "Drag and drop or click to choose photo"}</strong>
          <small>
            {selectedFile?.name
              ? "Click or drop another image to replace it"
              : "JPEG, PNG, or WebP, one image only"}
          </small>
          <input
            type="file"
            accept="image/jpeg, image/png, image/webp"
            onChange={handleFileChange}
          />
        </label>

        <label>
          Title
          <input
            type="text"
            name="title"
            value={title}
            onChange={handleTitleChange}
            placeholder="Morning light in Helsinki"
          />
        </label>

        <label>
          Short description
          <textarea
            name="description"
            value={description}
            onChange={handleDescriptionChange}
            placeholder="A few words about the moment"
            rows={4}
          />
        </label>

        <label>
          Date (UTC)
          <input
            type="date"
            name="date"
            min={minDate}
            max={maxDate}
            value={photoDate}
            onChange={handleDateChange}
            aria-invalid={!!dateError}
            aria-describedby={`photo-date-note${dateError ? " photo-date-error" : ""}`}
          />
          <small id="photo-date-note" className="photo-date-note">
            You can upload photos from the last seven days (one photo per day).
          </small>
          {dateError && (
            <span id="photo-date-error" className="auth-error">
              {dateError}
            </span>
          )}
        </label>

        <button
          type="button"
          disabled={!!dateError || !selectedFile || addPhoto.isPending}
          onClick={handleUpload}
        >
          {addPhoto.isPending ? "Adding photo..." : "Add photo"}
        </button>
        {uploadError && (
          <p className="auth-error" role="alert">
            {uploadError}
          </p>
        )}
      </form>
    </section>
  );
};

export default AddPhotoPage;
