import axios from "axios";
import { internalApi } from "./api";

const basePhotoURI = "/api/photos";

export type UploadIntentItem = {
  date: string; // expected format DD-MM-YYYY
  contentType: string;
  size: number;
  title: string;
  description: string;
};

type UploadIntentResponse = {
  photoID: string;
  url: string;
  method: string;
  formDataValues: Record<string, string>;
};

const uploadIntent = async (photoInfo: UploadIntentItem): Promise<UploadIntentResponse> => {
  const { data } = await internalApi.post<UploadIntentResponse>(
    `${basePhotoURI}/upload`,
    photoInfo,
  );

  return data;
};

const uploadFile = async (intent: UploadIntentResponse, file: File): Promise<void> => {
  const form = new FormData();

  for (const [key, value] of Object.entries(intent.formDataValues)) {
    form.append(key, value);
  }

  form.append("content-type", file.type);
  form.append("file", file);

  await axios.request({
    url: intent.url,
    method: "post",
    data: form,
    withCredentials: false,
  });
};

const completeUpload = async (photoID: string): Promise<void> => {
  await internalApi.post(`${basePhotoURI}/${photoID}/complete`);
};

const upload = async (photoInfo: UploadIntentItem, file: File): Promise<string> => {
  const photoIntentData = await uploadIntent(photoInfo);
  await uploadFile(photoIntentData, file);
  await completeUpload(photoIntentData.photoID);
  return photoIntentData.photoID;
};

const deletePhoto = async (photoID: string): Promise<void> => {
  await internalApi.delete(`${basePhotoURI}/${photoID}`);
};

const photoService = { upload, deletePhoto };

export default photoService;
