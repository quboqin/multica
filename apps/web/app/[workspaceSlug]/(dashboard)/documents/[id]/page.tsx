"use client";
import { use } from "react";
import { DocumentsPage } from "@multica/views/documents";
export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return <DocumentsPage documentId={id} />;
}
