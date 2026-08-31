"use client";

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Eye, FileText, ImageIcon, Pencil, Plus, Replace, Trash2, Upload } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys, creativeResourceFilesOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { attachmentDownloadPath, type CreativeResource, type CreativeResourceFile } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button, buttonVariants } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";
import { useT } from "../../i18n";
import { PRIME_TEMPLATE_SIZES, createDefaultPrimeTemplateSet, isPrimeTemplateRole } from "../lib/prime-template-set";

const PURPOSES = [
  "app_ui_reference",
  "brand_asset",
  "brand_guideline",
  "compliance_reference",
] as const;

type AssetDraft = { role: string; label: string; description: string; dimensions: string; market: string; locale: string; tags: string };
const EMPTY_DRAFT: AssetDraft = { role: "", label: "", description: "", dimensions: "", market: "Indonesia", locale: "id-ID", tags: "" };

export function MarketResourceFiles({ resource, value }: { resource: CreativeResource; value: Record<string, unknown> }) {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const { upload, uploading } = useFileUpload(api);
  const files = useQuery(creativeResourceFilesOptions(wsId, resource.id));
  const [editing, setEditing] = useState<CreativeResourceFile | "new" | null>(null);
  const [preview, setPreview] = useState<CreativeResourceFile | null>(null);
  const [uploadRole, setUploadRole] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const templateSet = createDefaultPrimeTemplateSet();
  const currentFiles = files.data?.files ?? [];
  const fileForRole = (role: string) => [...currentFiles].reverse().find((file) => file.role === role);
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: creativeKeys.resourceFiles(wsId, resource.id) });
    queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
  };
  const remove = useMutation({
    mutationFn: (fileId: string) => api.removeCreativeResourceFile(resource.id, fileId),
    onSuccess: () => { refresh(); toast.success(t(($) => $.resourceFiles.removed)); },
    onError: () => toast.error(t(($) => $.resourceFiles.removeFailed)),
  });
  const replaceTemplate = useMutation({
    mutationFn: async ({ role, selected, previous }: { role: string; selected: File; previous?: CreativeResourceFile }) => {
      const uploaded = await upload(selected);
      if (!uploaded) throw new Error(t(($) => $.resourceFiles.templateUploadFailed));
      const next = await api.addCreativeResourceFile(resource.id, {
        attachment_id: uploaded.id,
        role,
        label: selected.name,
        metadata: { market: textMetadata(value, "market"), locale: textMetadata(value, "locale"), source_type: "full_transparent_prime_template", dimensions: t(($) => $.resourceFiles.templateDimensions) },
      });
      if (previous) await api.removeCreativeResourceFile(resource.id, previous.id);
      return next;
    },
    onSuccess: () => { refresh(); toast.success(t(($) => $.resourceFiles.templateUpdated)); },
    onError: (error) => toast.error(error instanceof Error ? error.message : t(($) => $.resourceFiles.templateUpdateFailed)),
    onSettled: () => { setUploadRole(null); if (inputRef.current) inputRef.current.value = ""; },
  });
  const auxiliaryFiles = currentFiles.filter((file) => !isPrimeTemplateRole(file.role));
  return <section className="space-y-8">
    <input ref={inputRef} type="file" accept="image/png" className="hidden" onChange={(event) => {
      const selected = event.target.files?.[0];
      if (!selected || !uploadRole) return;
      replaceTemplate.mutate({ role: uploadRole, selected, previous: fileForRole(uploadRole) });
    }} />
    <section>
      <div className="mb-3"><h3 className="text-sm font-semibold">{t(($) => $.resourceFiles.primeTemplates)}</h3><p className="mt-1 text-xs text-muted-foreground">{t(($) => $.resourceFiles.primeTemplatesDescription)}</p></div>
      <div className="grid gap-4 lg:grid-cols-2">
        {templateSet.families.map((family) => <section key={family.id} className="border">
          <div className="border-b px-3 py-2"><p className="text-sm font-medium">{primeTemplateFamilyLabel(t, family.id)}</p><p className="mt-0.5 text-xs text-muted-foreground">{primeTemplateFamilyDescription(t, family.id)}</p></div>
          <div className="divide-y">{PRIME_TEMPLATE_SIZES.map(({ size }) => {
            const slot = family.templates[size];
            const file = fileForRole(slot.source_role);
            return <TemplateSlot t={t} key={slot.source_role} sizeLabel={`${primeTemplateSizeLabel(t, size)} · ${size}`} slot={slot} file={file} busy={replaceTemplate.isPending && uploadRole === slot.source_role} onPreview={() => file && setPreview(file)} onUpload={() => { setUploadRole(slot.source_role); inputRef.current?.click(); }} onRemove={() => file && remove.mutate(file.id)} />;
          })}</div>
        </section>)}
      </div>
    </section>
    <section>
      <div className="mb-3 flex flex-wrap items-end justify-between gap-3"><div><h3 className="text-sm font-semibold">{t(($) => $.resourceFiles.auxiliaryMaterials)}</h3><p className="mt-1 text-xs text-muted-foreground">{t(($) => $.resourceFiles.auxiliaryDescription)}</p></div><Button size="sm" variant="outline" onClick={() => setEditing("new")}><Plus className="h-4 w-4" />{t(($) => $.resourceFiles.addMaterial)}</Button></div>
      <div className="border-y">{auxiliaryFiles.map((file) => <ResourceFileRow t={t} key={file.id} file={file} onPreview={() => setPreview(file)} onEdit={() => setEditing(file)} onRemove={() => remove.mutate(file.id)} />)}{!files.isLoading && auxiliaryFiles.length === 0 && <div className="flex min-h-24 items-center justify-center text-sm text-muted-foreground">{t(($) => $.resourceFiles.noAuxiliary)}</div>}</div>
    </section>
    <ResourceFileDialog resource={resource} file={editing} uploading={uploading} upload={upload} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); refresh(); }} />
    <ResourcePreviewDialog file={preview} onClose={() => setPreview(null)} />
  </section>;
}

function TemplateSlot({ t, sizeLabel, slot, file, busy, onPreview, onUpload, onRemove }: { t: ReturnType<typeof useT>["t"]; sizeLabel: string; slot: { filename: string }; file?: CreativeResourceFile; busy: boolean; onPreview: () => void; onUpload: () => void; onRemove: () => void }) {
  const source = file ? resourceFileBrowserURL(file) : "";
  return <div className="grid grid-cols-[42px_minmax(0,1fr)_auto] items-center gap-2 px-3 py-2.5">
    <button type="button" disabled={!file} onClick={onPreview} className={cn("flex size-10 items-center justify-center overflow-hidden border disabled:cursor-default", imagePreviewBackground(file))} aria-label={file ? t(($) => $.resourceFiles.preview, { name: sizeLabel }) : t(($) => $.resourceFiles.notUploaded, { name: sizeLabel })}>{file && source ? <img src={source} alt="" className="h-full w-full object-contain" /> : <ImageIcon className="h-4 w-4 text-muted-foreground" />}</button>
    <div className="min-w-0"><p className="text-sm font-medium">{sizeLabel}</p><p className="truncate font-mono text-xs text-muted-foreground">{file ? file.label || file.filename : slot.filename}</p></div>
    <div className="flex items-center gap-1">{file && <Button size="icon-sm" variant="ghost" title={t(($) => $.resourceFiles.previewTemplate)} onClick={onPreview}><Eye className="h-4 w-4" /></Button>}<Button size="sm" variant="ghost" title={file ? t(($) => $.resourceFiles.replaceName, { name: sizeLabel }) : t(($) => $.resourceFiles.uploadName, { name: sizeLabel })} disabled={busy} onClick={onUpload}>{file ? <Replace className="h-4 w-4" /> : <Upload className="h-4 w-4" />}{file ? t(($) => $.resourceFiles.replace) : t(($) => $.resourceFiles.upload)}</Button>{file && <Button size="icon-sm" variant="ghost" title={t(($) => $.resourceFiles.removeTemplate)} onClick={onRemove}><Trash2 className="h-4 w-4" /></Button>}</div>
  </div>;
}

function ResourceFileRow({ t, file, onPreview, onEdit, onRemove }: { t: ReturnType<typeof useT>["t"]; file: CreativeResourceFile; onPreview: () => void; onEdit: () => void; onRemove: () => void }) {
  const source = resourceFileBrowserURL(file); const image = file.content_type.startsWith("image/"); const description = textMetadata(file.metadata, "description"); const dimensions = textMetadata(file.metadata, "dimensions");
  return <div className="grid gap-3 border-b px-3 py-3 last:border-b-0 sm:grid-cols-[72px_minmax(0,1fr)_auto] sm:items-center"><button type="button" onClick={onPreview} className="flex aspect-square w-[72px] items-center justify-center overflow-hidden border bg-muted/30" title={t(($) => $.resourceFiles.previewResource)}>{image && source ? <img src={source} alt={file.label || file.filename} className="h-full w-full object-contain" /> : <FileText className="h-5 w-5 text-muted-foreground" />}</button><div className="min-w-0"><div className="flex min-w-0 flex-wrap items-center gap-2"><p className="truncate text-sm font-medium">{file.label || file.filename}</p><Badge variant="outline" className="text-[10px]">{purposeLabel(t, file.role)}</Badge>{dimensions && <Badge variant="secondary" className="text-[10px]">{dimensions}</Badge>}</div><p className="mt-1 truncate text-xs text-muted-foreground">{description || file.filename}</p></div><div className="flex items-center justify-end gap-1"><Button size="icon-sm" variant="ghost" title={t(($) => $.resourceFiles.previewFile)} onClick={onPreview}><Eye className="h-4 w-4" /></Button>{source && <a href={source} download={file.filename} title={t(($) => $.resourceFiles.downloadFile)} className={buttonVariants({ size: "icon-sm", variant: "ghost" })}><Download className="h-4 w-4" /></a>}<Button size="icon-sm" variant="ghost" title={t(($) => $.resourceFiles.editFile)} onClick={onEdit}><Pencil className="h-4 w-4" /></Button><Button size="icon-sm" variant="ghost" title={t(($) => $.resourceFiles.removeFile)} onClick={onRemove}><Trash2 className="h-4 w-4" /></Button></div></div>;
}

function ResourceFileDialog({ resource, file, uploading, upload, onClose, onSaved }: { resource: CreativeResource; file: CreativeResourceFile | "new" | null; uploading: boolean; upload: (file: File) => Promise<{ id: string } | null>; onClose: () => void; onSaved: () => void }) {
  const { t } = useT("creative");
  const inputRef = useRef<HTMLInputElement>(null); const key = file === "new" ? "new" : file?.id ?? "closed"; const source = file && file !== "new" ? draftFromFile(file) : EMPTY_DRAFT; const [loadedKey, setLoadedKey] = useState(key); const [draft, setDraft] = useState<AssetDraft>(source); const [selectedFile, setSelectedFile] = useState<File | null>(null);
  useEffect(() => { if (key !== loadedKey) { setLoadedKey(key); setDraft(source); setSelectedFile(null); } }, [key, loadedKey, source]);
  const save = useMutation({ mutationFn: async () => { const metadata = { description: draft.description.trim(), dimensions: draft.dimensions.trim(), market: draft.market.trim(), locale: draft.locale.trim(), tags: splitTags(draft.tags) }; if (!file || !draft.role.trim()) throw new Error(t(($) => $.resourceFiles.selectPurpose)); if (file === "new") { if (!selectedFile) throw new Error(t(($) => $.resourceFiles.selectFile)); const uploaded = await upload(selectedFile); if (!uploaded) throw new Error(t(($) => $.resourceFiles.fileUploadFailed)); return api.addCreativeResourceFile(resource.id, { attachment_id: uploaded.id, role: draft.role.trim(), label: draft.label.trim() || selectedFile.name, metadata }); } const uploaded = selectedFile ? await upload(selectedFile) : null; if (selectedFile && !uploaded) throw new Error(t(($) => $.resourceFiles.fileUploadFailed)); return api.updateCreativeResourceFile(resource.id, file.id, { attachment_id: uploaded?.id, role: draft.role.trim(), label: draft.label.trim() || selectedFile?.name || file.filename, metadata }); }, onSuccess: () => { onSaved(); toast.success(file === "new" ? t(($) => $.resourceFiles.added) : selectedFile ? t(($) => $.resourceFiles.fileReplaced) : t(($) => $.resourceFiles.fileUpdated)); }, onError: (error) => toast.error(error instanceof Error ? error.message : t(($) => $.resourceFiles.saveFailed)) });
  const set = (field: keyof AssetDraft, next: string) => setDraft((current) => ({ ...current, [field]: next }));
  return <Dialog open={file !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-2xl"><DialogHeader><DialogTitle>{file === "new" ? t(($) => $.resourceFiles.addSupporting) : t(($) => $.resourceFiles.editMaterial)}</DialogTitle><DialogDescription>{t(($) => $.resourceFiles.dialogDescription)}</DialogDescription></DialogHeader><div className="grid gap-4 sm:grid-cols-2"><div className="sm:col-span-2"><input ref={inputRef} type="file" className="hidden" onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)} /><Button type="button" variant="outline" onClick={() => inputRef.current?.click()}><Upload className="h-4 w-4" />{selectedFile?.name || (file === "new" ? t(($) => $.resourceFiles.selectFile) : t(($) => $.resourceFiles.replaceFile))}</Button>{file && file !== "new" && !selectedFile && <span className="ml-3 align-middle text-xs text-muted-foreground">{file.filename}</span>}</div><AssetField label={t(($) => $.resourceFiles.purpose)}><NativeSelect value={draft.role} onChange={(event) => set("role", event.target.value)}><NativeSelectOption value="">{t(($) => $.resourceFiles.selectPurpose)}</NativeSelectOption>{draft.role && !PURPOSES.includes(draft.role as (typeof PURPOSES)[number]) && <NativeSelectOption value={draft.role}>{t(($) => $.resourceFiles.otherPurpose)}</NativeSelectOption>}{PURPOSES.map((purpose) => <NativeSelectOption key={purpose} value={purpose}>{purposeLabel(t, purpose)}</NativeSelectOption>)}</NativeSelect></AssetField><AssetField label={t(($) => $.resourceFiles.displayName)}><Input value={draft.label} onChange={(event) => set("label", event.target.value)} /></AssetField><AssetField label={t(($) => $.resourceFiles.dimensions)}><Input value={draft.dimensions} onChange={(event) => set("dimensions", event.target.value)} /></AssetField><AssetField label={t(($) => $.resourceFiles.market)}><Input value={draft.market} onChange={(event) => set("market", event.target.value)} /></AssetField><AssetField label={t(($) => $.resourceFiles.language)}><Input value={draft.locale} onChange={(event) => set("locale", event.target.value)} /></AssetField><AssetField label={t(($) => $.resourceFiles.tags)}><Input value={draft.tags} onChange={(event) => set("tags", event.target.value)} /></AssetField><AssetField label={t(($) => $.resourceFiles.usageNotes)} wide><Textarea rows={4} value={draft.description} onChange={(event) => set("description", event.target.value)} /></AssetField></div><DialogFooter><Button variant="outline" onClick={onClose}>{t(($) => $.resourceFiles.cancel)}</Button><Button disabled={uploading || save.isPending || !draft.role.trim() || (file === "new" && !selectedFile)} onClick={() => save.mutate()}>{selectedFile && file !== "new" ? t(($) => $.resourceFiles.replaceAndSave) : t(($) => $.resourceFiles.save)}</Button></DialogFooter></DialogContent></Dialog>;
}

function ResourcePreviewDialog({ file, onClose }: { file: CreativeResourceFile | null; onClose: () => void }) { const { t } = useT("creative"); const source = file ? resourceFileBrowserURL(file) : ""; const image = file?.content_type.startsWith("image/") === true; const pdf = file?.content_type === "application/pdf"; return <Dialog open={file !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-h-[94vh] gap-0 overflow-hidden p-0 sm:max-w-[min(94vw,1100px)]"><DialogHeader className="border-b px-4 py-3 pr-12"><DialogTitle className="truncate text-sm">{file?.label || file?.filename}</DialogTitle><DialogDescription className="truncate">{file?.filename}</DialogDescription></DialogHeader><div className={cn("flex h-[min(78vh,820px)] items-center justify-center p-3", image ? imagePreviewBackground(file) : "bg-muted/20")}>{source && image ? <img src={source} alt={file?.label || file?.filename} className="h-full w-full object-contain" /> : source && pdf ? <iframe src={source} title={file?.label || file?.filename} className="h-full w-full border-0 bg-background" /> : <div className="text-center"><ImageIcon className="mx-auto h-8 w-8 text-muted-foreground" /><p className="mt-3 text-sm text-muted-foreground">{t(($) => $.resourceFiles.noPreview)}</p>{source && <a href={source} download={file?.filename} className={cn(buttonVariants({ variant: "outline" }), "mt-4")}><Download className="h-4 w-4" />{t(($) => $.resourceFiles.downloadFile)}</a>}</div>}</div></DialogContent></Dialog>; }
function AssetField({ label, children, wide = false }: { label: string; children: React.ReactNode; wide?: boolean }) { return <div className={cn("space-y-1.5", wide && "sm:col-span-2")}><Label className="text-xs text-muted-foreground">{label}</Label>{children}</div>; }
function draftFromFile(file: CreativeResourceFile): AssetDraft { return { role: file.role, label: file.label, description: textMetadata(file.metadata, "description"), dimensions: textMetadata(file.metadata, "dimensions"), market: textMetadata(file.metadata, "market") || "Indonesia", locale: textMetadata(file.metadata, "locale") || "id-ID", tags: arrayMetadata(file.metadata, "tags").join(", ") }; }
function textMetadata(metadata: Record<string, unknown>, key: string): string { return typeof metadata[key] === "string" ? metadata[key] : ""; }
function arrayMetadata(metadata: Record<string, unknown>, key: string): string[] { return Array.isArray(metadata[key]) ? metadata[key].filter((value): value is string => typeof value === "string") : []; }
function splitTags(value: string): string[] { return value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean); }
export function resourceFileBrowserURL(file: CreativeResourceFile): string {
  return resolvePublicFileUrl(attachmentDownloadPath(file.attachment_id)) || resolvePublicFileUrl(file.url) || "";
}

export function imagePreviewBackground(file?: CreativeResourceFile | null): string {
  return file && (file.role.startsWith("prime_dark_") || textMetadata(file.metadata, "family") === "dark_background") ? "bg-black" : "bg-white";
}

export function purposeLabel(t: ReturnType<typeof useT>["t"], role: string): string {
  if (role === "app_ui_reference") return t(($) => $.resourceFiles.appUiReference);
  if (role === "brand_asset") return t(($) => $.resourceFiles.brandAsset);
  if (role === "brand_guideline") return t(($) => $.resourceFiles.brandGuideline);
  if (role === "compliance_reference") return t(($) => $.resourceFiles.complianceReference);
  return t(($) => $.resourceFiles.otherMaterial);
}

function primeTemplateSizeLabel(t: ReturnType<typeof useT>["t"], size: string): string {
  if (size === "1080x1080") return t(($) => $.comparison.square);
  if (size === "1200x628") return t(($) => $.comparison.landscape);
  if (size === "800x1000") return t(($) => $.comparison.portrait);
  return size;
}

function primeTemplateFamilyLabel(t: ReturnType<typeof useT>["t"], familyId: string): string {
  return familyId === "dark_background" ? t(($) => $.resourceFiles.family.darkTitle) : t(($) => $.resourceFiles.family.lightTitle);
}

function primeTemplateFamilyDescription(t: ReturnType<typeof useT>["t"], familyId: string): string {
  return familyId === "dark_background" ? t(($) => $.resourceFiles.family.darkDescription) : t(($) => $.resourceFiles.family.lightDescription);
}
