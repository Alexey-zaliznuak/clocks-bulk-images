export interface VKCabinet {
  baseUrl?: string;
  sudo?: string;
}

// VKObjectRef is the path of one object in the cabinet. The dashboard nests
// them, so a group is only addressable inside its plan and an ad inside its
// group.
export interface VKObjectRef {
  planId?: string;
  groupId?: string;
  adId?: string;
}

// vkObjectUrl points at the dashboard page of an uploaded object. Agency
// tokens see a client account, and the cabinet only opens it with the sudo
// switch — without it the link lands on the agency dashboard instead.
export function vkObjectUrl(cabinet: VKCabinet | null, ref: VKObjectRef): string {
  const base = (cabinet?.baseUrl || "").replace(/\/+$/, "");
  if (!base || !ref.planId) return "";
  let path = `/hq/dashboard/plans/${encodeURIComponent(ref.planId)}`;
  if (ref.groupId) {
    path += `/groups/${encodeURIComponent(ref.groupId)}`;
    if (ref.adId) path += `/ads/${encodeURIComponent(ref.adId)}`;
  }
  const url = base + path;
  return cabinet?.sudo ? `${url}?sudo=${encodeURIComponent(cabinet.sudo)}` : url;
}
