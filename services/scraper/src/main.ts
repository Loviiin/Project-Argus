import { connect, StringCodec, NatsConnection, AckPolicy } from "nats";
import { ScraperEngine } from "./engine";

const sc = StringCodec();

async function bootstrap() {
  console.log('Scraper Worker iniciando');

  const engine = new ScraperEngine();
  await engine.init();

  let nc: NatsConnection | undefined;

  try {
    const streamName = "OSINT_PIPELINE";
    const streamSubjects = ["jobs.>"];
    const consumerName = "SCRAPER_WORKER";
    const filterSubject = "jobs.scrape";

    nc = await connect({ servers: "nats://localhost:4222" });
    console.log(`Conectado ao NATS em: ${nc.getServer()}`);

    const jsm = await nc.jetstreamManager();

    await jsm.streams.add({ name: streamName, subjects: streamSubjects });
    console.log(`Stream [${streamName}] verificada.`);

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
        const data = sc.decode(m.data);
        console.log(`\nMENSAGEM RECEBIDA (Seq: ${m.seq}):`);
        console.log(` Conteúdo: ${data}`);

        console.log("Trabalhando");

        const savePath = await engine.processUrl(data, m.seq.toString());
        console.log(`Processamento concluído. Evidência salva em: ${savePath}`);

        await js.publish("jobs.analyse", sc.encode(savePath));
        console.log("Evidência enviada para análise.");

        m.ack();
        console.log("Ack enviado.");

      } catch (err) {
        console.error("Erro processando mensagem:", err);
      }
    }

  } catch (err) {
    console.error('Erro fatal:', err);
    process.exit(1);
  }

  process.on('SIGINT', async () => {
    console.log('Encerrando conexões');
    if (nc) await nc.drain();
    process.exit(0);
  });
}

bootstrap();