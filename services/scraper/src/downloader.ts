import { v4 as uuidv4 } from "uuid";
import path from "path";
import fs from "fs";

const youtubedl = require("youtube-dl-exec");

const COOKIES_PATH = path.resolve(__dirname, "../../../config/cookies.txt");

function getOptions(baseOptions: any) {
  const options = {
    ...baseOptions,
    userAgent:
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
  };

  if (fs.existsSync(COOKIES_PATH)) {
    console.log(`Usando cookies de: ${COOKIES_PATH}`);
    options.cookies = COOKIES_PATH;
  } else {
    console.log("Arquivo de cookies não encontrado, prosseguindo sem cookies.");
  }
  return options;
}

export async function extrairMetadados(url: string) {
  console.log(`Extraindo metadados de ${url}`);
  try {
    const output = await youtubedl(
      url,
      getOptions({
        dumpJson: true,
        noCheckCertificate: true,
        noWarnings: true,
        preferFreeFormats: true,
        getComments: true,
      }),
    );

    return output;
  } catch (error) {
    console.error(`Erro ao extrair metadados:`, error);
    return null;
  }
}

export async function baixavideo(url: string) {
  const outputDir = path.resolve(__dirname, "../../../tmp_data");
  const id = uuidv4();
  const saida = path.join(outputDir, `${id}.mp4`);

  if (!fs.existsSync(outputDir)) {
    fs.mkdirSync(outputDir, { recursive: true });
  }

  console.log(`Baixando vídeo de ${url} com ID ${id}`);

  const startTime = Date.now();
  try {
    await youtubedl(
      url,
      getOptions({
        output: saida,
        format: "mp4",
        noCheckCertificate: true,
        noWarnings: true,
        preferFreeFormats: true,
      }),
    );

    const duration = (Date.now() - startTime) / 1000;
    console.log(`Download concluído em ${duration}s: ${saida}`);
    return saida;
  } catch (error) {
    console.error(`Erro ao baixar vídeo:`, error);
    return null;
  }
}
