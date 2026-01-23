import { connect, StringCodec } from "nats";

(async () => {
    const nc = await connect({ servers: "nats://localhost:4222" });
    const js = nc.jetstream();
    const sc = StringCodec();

    await js.publish("jobs.scrape", sc.encode("https://www.youtube.com/watch?v=dQw4w9WgXcQ"));
    
    console.log("🚀 URL enviada para a fila!");
    await nc.drain();
})();