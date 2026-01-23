import { connect, StringCodec } from "nats";

(async () => {
    const nc = await connect({ servers: "nats://localhost:4222" });
    const js = nc.jetstream();
    const sc = StringCodec();

    const target = "https://youtube.com/watch?v=dQw4w9WgXcQ";

    console.log(`Iniciando pipeline para: ${target}`);
    await js.publish("jobs.scrape", sc.encode(target));
    
    await nc.drain();
})();