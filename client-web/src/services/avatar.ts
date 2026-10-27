import axios from "axios";
import { internalApi } from "./api";

const baseAvatarURI = "/api/users/me/avatar";

export type UploadAvatarIntentItem = {
  contentType: string;
  size: number;
};

type UploadAvatarIntentResponse = {
  uploadID: string;
  url: string;
  method: string;
  formDataValues: Record<string, string>;
};

const uploadIntent = async (
  avatarInfo: UploadAvatarIntentItem,
): Promise<UploadAvatarIntentResponse> => {
  const { data } = await internalApi.post<UploadAvatarIntentResponse>(
    `${baseAvatarURI}/uploads`,
    avatarInfo,
  );

  return data;
};

const uploadFile = async (intent: UploadAvatarIntentResponse, file: File): Promise<void> => {
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

const completeUpload = async (uploadID: string): Promise<void> => {
  await internalApi.post(`${baseAvatarURI}/uploads/${uploadID}/complete`);
};

const upload = async (avatarInfo: UploadAvatarIntentItem, file: File): Promise<void> => {
  const intentData = await uploadIntent(avatarInfo);
  await uploadFile(intentData, file);
  await completeUpload(intentData.uploadID);
};

const deleteAvatar = async (): Promise<void> => {
  await internalApi.delete(`${baseAvatarURI}`);
};

const avatarService = { upload, deleteAvatar };

export default avatarService;
