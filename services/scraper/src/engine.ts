import { chromium, Browser, Page } from "playwright";
import path from "path";

export class ScraperEngine {
  private browser: Browser | null = null;

  async init() {
    console.log("Inicializando Engine do Playwright...");
    this.browser = await chromium.launch({
      headless: true,
      args: ["--no-sandbox", "--disable-setuid-sandbox"],
    });
  }

  async processUrl(url: string, jobId: string): Promise<string> {
    if (!this.browser) throw new Error("Browser não iniciado!");
    const context = await this.browser.newContext({
      userAgent:
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
    });

    const page = await context.newPage();

    try {
      console.log(`Navegando para: ${url}`);
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: 70000 });
      const filename = `job_${jobId}_${Date.now()}.png`;
      const savePath = path.resolve(__dirname, "../../../tmp_data", filename);

      console.log("Aguardando carregamento da página...");

      await page.waitForTimeout(5000);

      await page.screenshot({ path: savePath });
      console.log(`Evidência salva em: ${savePath}`);
      return savePath;
    } catch (error) {
      console.error(`Falha ao processar ${url}`);
      throw error;
    } finally {
      await context.close();
    }
  }

  async close() {
    if (this.browser) await this.browser.close();
  }
}
