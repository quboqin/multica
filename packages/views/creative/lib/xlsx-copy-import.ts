import { strFromU8, unzipSync } from "fflate";

export interface SpreadsheetData {
  headers: string[];
  rows: string[][];
  sheetName: string;
}

export async function readCopySpreadsheet(file: File): Promise<SpreadsheetData> {
  return (await readSpreadsheetWorkbook(file))[0] ?? { headers: [], rows: [], sheetName: "Sheet 1" };
}

export async function readSpreadsheetWorkbook(file: File): Promise<SpreadsheetData[]> {
  if (file.name.toLowerCase().endsWith(".csv")) {
    return [rowsToSpreadsheet(parseCsv(await file.text()), "CSV")];
  }
  if (!file.name.toLowerCase().endsWith(".xlsx")) {
    throw new Error("Only .xlsx and .csv files are supported");
  }
  const bytes = new Uint8Array(await file.arrayBuffer());
  const hasZipSignature = bytes.length >= 4
    && bytes[0] === 0x50
    && bytes[1] === 0x4b
    && (bytes[2] === 0x03 || bytes[2] === 0x05 || bytes[2] === 0x07)
    && (bytes[3] === 0x04 || bytes[3] === 0x06 || bytes[3] === 0x08);
  if (!hasZipSignature) {
    throw new Error("文件内容不是有效的 Excel 工作簿；请检查是否下载成了登录页或错误页");
  }
  const files = unzipSync(bytes);
  const workbook = parseXml(readArchiveText(files, "xl/workbook.xml"));
  const relationships = parseXml(readArchiveText(files, "xl/_rels/workbook.xml.rels"));
  const sharedStrings = readSharedStrings(files);
  const sheets = Array.from(workbook.getElementsByTagName("sheet"));
  if (sheets.length === 0) throw new Error("The workbook has no sheets");
  return sheets.map((sheet) => {
    const relationshipId = sheet.getAttribute("r:id") ?? "";
    const relationship = Array.from(relationships.getElementsByTagName("Relationship"))
      .find((item) => item.getAttribute("Id") === relationshipId);
    const target = relationship?.getAttribute("Target");
    if (!target) throw new Error(`Missing workbook relationship for sheet: ${sheet.getAttribute("name") ?? "unknown"}`);
    const worksheetPath = target.startsWith("/")
      ? target.replace(/^\//, "")
      : `xl/${target.replace(/^\.\//, "")}`;
    const worksheet = parseXml(readArchiveText(files, worksheetPath));
    const rows = Array.from(worksheet.getElementsByTagName("row")).map((row) => {
      const values: string[] = [];
      for (const cell of Array.from(row.getElementsByTagName("c"))) {
        const reference = cell.getAttribute("r") ?? "A1";
        const columnIndex = columnIndexFromReference(reference);
        values[columnIndex] = readCellValue(cell, sharedStrings);
      }
      return values.map((value) => value ?? "");
    });
    return rowsToSpreadsheet(rows, sheet.getAttribute("name") ?? "Sheet 1");
  });
}

function readArchiveText(files: Record<string, Uint8Array>, path: string): string {
  const value = files[path.replace(/\\/g, "/")];
  if (!value) throw new Error(`Missing workbook part: ${path}`);
  return strFromU8(value);
}

function parseXml(value: string): Document {
  const document = new DOMParser().parseFromString(value, "application/xml");
  if (document.getElementsByTagName("parsererror").length > 0) {
    throw new Error("The workbook contains invalid XML");
  }
  return document;
}

function readSharedStrings(files: Record<string, Uint8Array>): string[] {
  const bytes = files["xl/sharedStrings.xml"];
  if (!bytes) return [];
  const document = parseXml(strFromU8(bytes));
  return Array.from(document.getElementsByTagName("si")).map((item) =>
    Array.from(item.getElementsByTagName("t")).map((text) => text.textContent ?? "").join(""),
  );
}

function readCellValue(cell: Element, sharedStrings: string[]): string {
  const type = cell.getAttribute("t") ?? "";
  if (type === "inlineStr") {
    return Array.from(cell.getElementsByTagName("t")).map((text) => text.textContent ?? "").join("");
  }
  const raw = cell.getElementsByTagName("v")[0]?.textContent ?? "";
  if (type === "s") return sharedStrings[Number(raw)] ?? "";
  if (type === "b") return raw === "1" ? "TRUE" : "FALSE";
  return raw;
}

function columnIndexFromReference(reference: string): number {
  const letters = reference.match(/^[A-Z]+/i)?.[0]?.toUpperCase() ?? "A";
  let value = 0;
  for (const letter of letters) value = value * 26 + letter.charCodeAt(0) - 64;
  return Math.max(0, value - 1);
}

function rowsToSpreadsheet(rows: string[][], sheetName: string): SpreadsheetData {
  const meaningful = rows.filter((row) => row.some((value) => value.trim() !== ""));
  const headers = (meaningful[0] ?? []).map((value, index) => value.trim() || `Column ${index + 1}`);
  const dataRows = meaningful.slice(1).map((row) => headers.map((_, index) => row[index]?.trim() ?? ""));
  if (headers.length === 0) throw new Error("The spreadsheet has no header row");
  return { headers, rows: dataRows, sheetName };
}

function parseCsv(value: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (quoted && character === '"' && value[index + 1] === '"') {
      cell += '"';
      index += 1;
    } else if (character === '"') {
      quoted = !quoted;
    } else if (!quoted && character === ",") {
      row.push(cell);
      cell = "";
    } else if (!quoted && (character === "\n" || character === "\r")) {
      if (character === "\r" && value[index + 1] === "\n") index += 1;
      row.push(cell);
      rows.push(row);
      row = [];
      cell = "";
    } else {
      cell += character;
    }
  }
  row.push(cell);
  rows.push(row);
  return rows;
}
