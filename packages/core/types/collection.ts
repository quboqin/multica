export type CollectionFieldType = "text" | "number" | "checkbox";

export interface Collection {
  id: string;
  workspaceId: string;
  name: string;
  revision: number;
  archivedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface CollectionField {
  id: string;
  workspaceId: string;
  collectionId: string;
  name: string;
  type: string;
  position: number;
  revision: number;
}

export interface CollectionRecord {
  id: string;
  workspaceId: string;
  collectionId: string;
  title: string;
  fields: Record<string, unknown>;
  position: number;
  revision: number;
  createdAt: string;
  updatedAt: string;
}

export interface CollectionCapabilities {
  layouts: string[];
  grouping: boolean;
  hierarchy: boolean;
  writable: boolean;
  maxPageSize: number;
}

export interface CollectionDetail {
  collection: Collection;
  fields: CollectionField[];
  capabilities: CollectionCapabilities;
}

export interface CollectionPage {
  collections: Collection[];
  total: number;
  nextCursor: string | null;
}

export interface CollectionRecordPage {
  records: CollectionRecord[];
  total: number;
  nextCursor: string | null;
}

export interface CreateCollectionInput {
  clientRequestId: string;
  name: string;
  fields: Array<{ name: string; type: CollectionFieldType }>;
}

export interface CreateCollectionResult {
  collection: Collection;
  fields: CollectionField[];
  replayed: boolean;
}

export interface CreateCollectionRecordInput {
  clientRequestId: string;
  title: string;
  fields: Record<string, unknown>;
}

export interface CreateCollectionRecordResult {
  record: CollectionRecord;
  replayed: boolean;
}

export interface UpdateCollectionRecordInput {
  expectedRevision: number;
  change:
    | { fieldId: "title"; op: "set"; value: string }
    | { fieldId: string; op: "set"; value: unknown }
    | { fieldId: string; op: "clear" };
}
