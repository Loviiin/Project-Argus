import asyncio
import json
import nats
from nats.errors import TimeoutError

async def main():
    nc = await nats.connect("nats://localhost:4222")
    js = nc.jetstream()

    print("--- Connected to NATS ---")

    scraper_input = "jobs.scrape"
    parser_output = "data.text_extracted"
    vision_output = "jobs.analyse"

    sub_parser = await nc.subscribe(parser_output)
    sub_vision = await nc.subscribe(vision_output)

    video_url = "https://www.youtube.com/shorts/IknOw-k2nB0"
    print(f"\n[1] Publishing to '{scraper_input}': {video_url}")
    await js.publish(scraper_input, video_url.encode())

    print(f"\n[2] Waiting for metadata on '{parser_output}'...")
    try:
        # Aumentado para 600s para dar mais tempo de processamento
        msg = await sub_parser.next_msg(timeout=600)
        data = json.loads(msg.data.decode())
        print(f"✅ Received Metadata:")
        print(f"   - Source: {data.get('source_path')}")
        print(f"   - Text Length: {len(data.get('text_content', ''))} chars")
        print(f"   - Type: {data.get('source_type')}")
    except TimeoutError:
        print(f"❌ Timeout waiting for metadata on '{parser_output}'.")
        print("   ⚠️  Possíveis causas:")
        print("   1. O serviço 'parser' pode não estar rodando ou travou.")
        print("   2. O scraper falhou em baixar o vídeo ou extrair a descrição.")
        print("   3. O NATS não está roteando a mensagem corretamente.")

    print(f"\n[3] Waiting for video job on '{vision_output}' (this includes download time)...")
    try:
        msg = await sub_vision.next_msg(timeout=120) 
        data = json.loads(msg.data.decode())
        print(f"✅ Received Video Job:")
        print(f"   - Path: {data.get('source_path')}")
        print(f"   - Author: {data.get('author_id')}")
    except TimeoutError:
        print("❌ Timeout waiting for video job (download might be too slow).")

    await nc.close()

if __name__ == '__main__':
    asyncio.run(main())
