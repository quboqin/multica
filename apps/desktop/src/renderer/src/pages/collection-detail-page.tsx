import { useParams } from "react-router-dom";
import { CollectionDetailPage as CollectionDetailPageView } from "@multica/views/collections";

export function CollectionDetailPage() {
  const { id } = useParams<{ id: string }>();
  if (!id) return null;
  return <CollectionDetailPageView collectionId={id} />;
}
