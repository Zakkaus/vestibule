export function groupName(id: string, title?: string): string {
  return title && title.trim() ? title : id;
}
