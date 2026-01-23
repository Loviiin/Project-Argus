import { connect, StringCodec } from "nats";

(async () => {
  const nc = await connect({ servers: "nats://localhost:4222" });
  const js = nc.jetstream();
  const sc = StringCodec();

  const target2 = "https://vt.tiktok.com/ZSaSxMWSC/";
  const target = "https://youtube.com/watch?v=dQw4w9WgXcQ";

  console.log(`Iniciando pipeline para: ${target2}`);
  await js.publish("jobs.scrape", sc.encode(target2));

  await nc.drain();
})();
