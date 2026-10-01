export class ReportBundle {
  private files: Record<string, Uint8Array> = {};
  private bytes = 0;
  add(name: string, bytes: Uint8Array): void {
    if (!name || /[\\/:]/.test(name) || Object.hasOwn(this.files, name))
      throw new Error("Invalid or duplicate report filename");
    if (this.bytes + bytes.length > 190 * 1024 * 1024)
      throw new Error(
        "Report bundle exceeds 190 MiB / 報表包過大，請縮小查詢範圍",
      );
    this.bytes += bytes.length;
    this.files[name] = bytes;
  }
  async build(): Promise<Uint8Array> {
    if (!Object.keys(this.files).length)
      throw new Error("No report files / 沒有報表可匯出");
    const { zip } = await import("fflate");
    return new Promise((resolve, reject) =>
      zip(this.files, { level: 0 }, (error, data) =>
        error ? reject(error) : resolve(data),
      ),
    );
  }
}
