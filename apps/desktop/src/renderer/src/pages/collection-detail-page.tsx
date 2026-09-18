import { useParams } from "react-router-dom";
import { CollectionDetailPage as SharedCollectionDetailPage } from "@multica/views/collections";
export function CollectionDetailPage() {
  const { id } = useParams();
  return id ? <SharedCollectionDetailPage id={id} /> : null;
}
