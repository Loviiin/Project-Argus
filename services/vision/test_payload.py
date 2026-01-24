import asyncio
import nats
import json


async def main():
    nc = await nats.connect("nats://localhost:4222")
    js = nc.jetstream()

    dados_fake = {
        "source_path": "teste_manual_debug.png",
        "text_content": "Fala galera, entrem no meu servidor: discord.gg/devs e também no https://discord.gg/C4ydNXTt valeu!"
    }

    payload_bytes = json.dumps(dados_fake).encode()

    await js.publish("data.text_extracted", payload_bytes)
    print("Payload manual enviado para 'data.text_extracted'!")

    await nc.drain()

if __name__ == '__main__':
    asyncio.run(main())
