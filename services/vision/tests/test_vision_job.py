import asyncio
import nats
import json
import os

async def main():
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    try:
        nc = await nats.connect(nats_url)
        js = nc.jetstream()
    except Exception as e:
        print(f"Erro ao conectar no NATS: {e}")
        return

    # Garante stream
    try:
        await js.add_stream(name="JOBS", subjects=["jobs.>"])
    except:
        pass

    # Caminho do vídeo para teste
    # Você deve colocar um arquivo de vídeo real aqui para testar o OCR
    video_path = os.path.abspath("test_video.mp4")
    
    # Cria um arquivo dummy só para não dar erro de "file not found" imediato
    # O ff/cv2 vai reclamar se for vazio, mas o fluxo do código será testado.
    if not os.path.exists(video_path):
        with open(video_path, "wb") as f:
            f.write(b"dummy content")
        print(f"Arquivo dummy criado em: {video_path} (substitua por um video real)")

    payload = {
        "source_path": video_path,
        "author_id": "metadado_teste"
    }

    await js.publish("jobs.analyse", json.dumps(payload).encode())
    print(f"Job enviado para 'jobs.analyse' com path: {video_path}")

    await nc.drain()

if __name__ == '__main__':
    asyncio.run(main())
