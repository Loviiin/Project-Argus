import { connect, StringCodec } from "nats";

(async () => {
  const natsUrl = process.env.NATS_URL || "nats://localhost:4222";
  const nc = await connect({ servers: natsUrl });
  const js = nc.jetstream();
  const sc = StringCodec();

  const target3 =
    "https://www.reddit.com/r/discordapp/comments/icwt62/is_there_anyway_to_change_what_url_the_message/?tl=pt-br";
  const target2 = "https://vt.tiktok.com/ZSaSxMWSC/";
  const target = "https://youtube.com/watch?v=dQw4w9WgXcQ";

  console.log(`Iniciando pipeline para: ${target3}`);
  await js.publish("jobs.scrape", sc.encode(target3));

  await nc.drain();
})();
