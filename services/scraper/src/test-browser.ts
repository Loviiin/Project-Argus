import { chromium } from 'playwright';
import path from 'path';

(async () => {
  console.log('Iniciando teste do browser headless');

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto('https://www.google.com');
    
    const title = await page.title();
    console.log(`Título capturado: ${title}`);

    const screenshotPath = path.resolve(__dirname, '../../../tmp_data/prova_de_vida.png');
    
    await page.screenshot({ path: screenshotPath });
    
    console.log(`Screenshot salvo em: ${screenshotPath}`);
  } catch (error) {
    console.error('Erro:', error);
  } finally {
    await browser.close();
    console.log('Processo finalizado.');
  }
})();