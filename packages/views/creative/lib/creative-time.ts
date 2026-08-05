const CREATIVE_TIME_ZONE = "Asia/Shanghai";

const creativeDateTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  timeZone: CREATIVE_TIME_ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

export function formatCreativeDateTime(value: string): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : creativeDateTimeFormatter.format(date);
}

export function creativeTimeZoneLabel(): string {
  return "北京时间";
}
