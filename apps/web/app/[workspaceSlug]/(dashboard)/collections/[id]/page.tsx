"use client";

import { use } from "react";
import { CollectionDetailPage } from "@multica/views/collections";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return <CollectionDetailPage collectionId={id} />;
}
