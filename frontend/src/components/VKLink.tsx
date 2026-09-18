import { vkObjectUrl, type VKCabinet, type VKObjectRef } from "../vkCabinet";

// VKLink renders a cabinet link when the object exists and the cabinet is
// reachable, and the plain label otherwise, so the page reads the same before
// the upload has happened.
export default function VKLink({
  cabinet,
  planId,
  groupId,
  adId,
  label,
  fallback = "—",
  className = "",
}: VKObjectRef & {
  cabinet: VKCabinet | null;
  label?: string;
  fallback?: string;
  className?: string;
}) {
  const text = label || adId || groupId || planId || "";
  if (!text) return <span className={className}>{fallback}</span>;
  const href = vkObjectUrl(cabinet, { planId, groupId, adId });
  if (!href) return <span className={className}>{text}</span>;
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      title="Открыть в кабинете VK Рекламы"
      className={`text-blue-600 underline decoration-blue-300 underline-offset-2 hover:text-blue-700 ${className}`}
    >
      {text}
    </a>
  );
}
