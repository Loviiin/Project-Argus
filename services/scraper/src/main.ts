import {
  connect,
  StringCodec,
  JSONCodec,
  NatsConnection,
  AckPolicy,
} from "nats";
import fs from "fs";
import path from "path";
import { ScraperEngine } from "./engine";
import { baixavideo, extrairMetadados } from "./downloader";

const sc = StringCodec();
const jc = JSONCodec();

async function bootstrap() {
  console.log("Scraper Worker iniciando");

  const engine = new ScraperEngine();
  await engine.init();

  let nc: NatsConnection | undefined;

  try {
    const streamName = "OSINT_PIPELINE";
    const streamSubjects = ["jobs.>", "data.>"];
    const consumerName = "SCRAPER_WORKER";
    const filterSubject = "jobs.scrape";

    const natsUrl = process.env.NATS_URL || "nats://localhost:4222";
    nc = await connect({ servers: natsUrl });
    console.log(`Conectado ao NATS em: ${nc.getServer()}`);

    const jsm = await nc.jetstreamManager();

    try {
      await jsm.streams.info(streamName);
      console.log(`Stream [${streamName}] encontrada. Atualizando subjects...`);
      await jsm.streams.update(streamName, { subjects: streamSubjects });
      console.log(`Stream [${streamName}] atualizada com sucesso.`);
    } catch (e) {
      console.log(`Stream [${streamName}] não encontrada. Criando...`);
      await jsm.streams.add({ name: streamName, subjects: streamSubjects });
      console.log(`Stream [${streamName}] criada.`);
    }

    await jsm.consumers.add(streamName, {
      durable_name: consumerName,
      filter_subject: filterSubject,
      ack_policy: AckPolicy.Explicit,
    });
    console.log(`Consumer [${consumerName}] registrado.`);

    const js = nc.jetstream();
    const consumer = await js.consumers.get(streamName, consumerName);

    console.log(`Ouvindo fila [${filterSubject}]...`);

    const messages = await consumer.consume();

    for await (const m of messages) {
      try {
        const url = sc.decode(m.data);
        console.log(`\nMENSAGEM RECEBIDA (Seq: ${m.seq}):`);
        console.log(` Conteúdo: ${url}`);

        console.log("Trabalhando");

        const metadata = await extrairMetadados(url);
        if (metadata) {
          let textContent = metadata.description || "Sem descrição";

          if (metadata.comments && Array.isArray(metadata.comments)) {
            const commentsText = metadata.comments
              .map((c: any) => `[Comentário de ${c.author}]: ${c.text}`)
              .join("\n");
            textContent += `\n\n=== COMENTÁRIOS ===\n${commentsText}`;
          }

          const payloadTexto = {
            source_path: url,
            text_content: textContent,
            source_type: "metadata_comments",
          };
          await js.publish("data.text_extracted", jc.encode(payloadTexto));
          console.log(
            "Metadados e comentários enviados para data.text_extracted",
          );

          try {
            const logPath = path.resolve(
              __dirname,
              "../../../tmp_data/historico_texto.txt",
            );
            if (!fs.existsSync(path.dirname(logPath))) {
              fs.mkdirSync(path.dirname(logPath), { recursive: true });
            }
            const timestamp = new Date().toISOString();
            const logContent = `\n--- [${timestamp}] Job: ${url} ---\n${textContent}\n`;
            fs.appendFileSync(logPath, logContent);
            console.log(`Texto salvo em ${logPath}`);
          } catch (e) {
            console.error("Erro ao salvar log de texto:", e);
          }
        } else {
          console.log("Falha ao extrair metadados, pulando etapa de texto.");
        }

        const caminhoVideo = await baixavideo(url);
        if (caminhoVideo) {
          const author = metadata?.uploader_id
            ? `@${metadata.uploader_id}`
            : "@desconhecido";
          const payloadVision = {
            source_path: caminhoVideo,
            author_id: author,
          };

          await js.publish("jobs.analyse", jc.encode(payloadVision));
          console.log("Vídeo enviado para jobs.analyse");
        } else {
          console.log("Falha ao baixar o vídeo.");
        }

        m.ack();
        console.log("Ack enviado.");
      } catch (err) {
        console.error("Erro processando mensagem:", err);
      }
    }
  } catch (err) {
    console.error("Erro fatal:", err);
    process.exit(1);
  }

  process.on("SIGINT", async () => {
    console.log("Encerrando conexões");
    if (nc) await nc.drain();
    process.exit(0);
  });
}

bootstrap();
