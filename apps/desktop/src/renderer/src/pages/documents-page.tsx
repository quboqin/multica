import { useParams } from "react-router-dom";
import { DocumentsPage as SharedDocumentsPage } from "@multica/views/documents";
export function DocumentsPage() {
  const { id } = useParams();
  return <SharedDocumentsPage documentId={id} />;
}
