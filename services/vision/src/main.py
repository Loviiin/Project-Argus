import asyncio
import json
import os
import datetime
import signal
import sys
import cv2
import easyocr
import numpy as np
import nats
from nats.errors import ConnectionClosedError, TimeoutError, NoRespondersError
from captcha_solver import solve_captcha

print("Carregando modelo OCR (pode demorar um pouco)...")
reader = easyocr.Reader(['en', 'pt'], gpu=True) 
print("Modelo OCR carregado!")

def preprocess_frame(frame):
    gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
    
    clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8,8))
    enhanced = clahe.apply(gray)
    
    return enhanced

async def process_video(video_path):
    print(f"Processando vídeo: {video_path}")
    
    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        print(f"Erro: Não foi possível abrir o vídeo {video_path}")
        return ""

    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    fps = cap.get(cv2.CAP_PROP_FPS)
    if fps == 0 or total_frames == 0:
        print("Erro: Vídeo vazio ou inválido")
        return ""

    points = [0.3, 0.5, 0.9]
    full_text = []

    for p in points:
        frame_id = int(total_frames * p)
        cap.set(cv2.CAP_PROP_POS_FRAMES, frame_id)
        ret, frame = cap.read()
        
        if ret:
            processed_frame = preprocess_frame(frame)
            results = reader.readtext(processed_frame, detail=0)
            text_chunk = " ".join(results)
            full_text.append(f"[FRAME_{int(p*100)}%]: {text_chunk}")
        else:
            print(f"Falha ao ler frame no ponto {p}")

    cap.release()
    
    final_text = " ".join(full_text)
    print(f"OCR Concluído. Texto extraído (resumo): {final_text[:100]}...")
    return final_text

async def main():
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    
    print(f"Conectando ao NATS em {nats_url}...")
    
    nc = await nats.connect(nats_url)
    js = nc.jetstream()
    
    print("Vision Service (Python) iniciado. Aguardando vídeos...")

    async def message_handler(msg):
        subject = msg.subject
        try:
            data = json.loads(msg.data.decode())
        except json.JSONDecodeError:
            print("Erro: Payload inválido (não é JSON). Ignorando mensagem.")
            await msg.ack()
            return

        video_path = data.get("source_path")
        
        await msg.in_progress()

        if not video_path:
            print("Job sem source_path. Ignorando.")
            await msg.ack()
            return

        if not os.path.exists(video_path):
             print(f"Arquivo não encontrado: {video_path}")
             await msg.ack()
             return

        try:
            extracted_text = await process_video(video_path)

            if extracted_text:
                try:
                    log_path = os.path.abspath("../../tmp_data/historico_ocr.txt")
                    os.makedirs(os.path.dirname(log_path), exist_ok=True)
                    timestamp = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
                    with open(log_path, "a", encoding="utf-8") as f:
                        f.write(f"\n--- [{timestamp}] Job: {video_path} ---\n{extracted_text}\n")
                    print(f"Texto salvo em {log_path}")
                except Exception as e_log:
                    print(f"Falha ao salvar log local: {e_log}")
            
            payload = {
                "source_path": video_path,
                "text_content": extracted_text,
                "author_id": data.get("author_id", "desconhecido"),
                "source_type": "video_ocr",
                "metadata": {
                    "engine": "easyocr",
                    "version": "1.0"
                }
            }

            await js.publish("data.text_extracted", json.dumps(payload).encode())
            print(f"Resultado enviado para data.text_extracted")
            
            await msg.ack()

        except Exception as e:
            print(f"Erro processando {video_path}: {e}")
            await msg.nak()

    await js.subscribe("jobs.analyse", cb=message_handler, durable="vision-worker")

    # Handler para resolver captchas
    async def captcha_handler(msg):
        try:
            data = json.loads(msg.data.decode())
            print(f"[Captcha] Recebida requisição de captcha")
            
            # Resolve o captcha
            result = solve_captcha(data)
            
            # Envia resposta
            response = json.dumps(result).encode()
            await msg.respond(response)
            
            status = "✓" if result.get('success') else "✗"
            print(f"[Captcha] {status} Resposta enviada: {result.get('distance_x', 0):.2f}px")
            
        except Exception as e:
            print(f"[Captcha] Erro: {e}")
            error_response = {
                'success': False,
                'distance_x': 0,
                'error': str(e)
            }
            await msg.respond(json.dumps(error_response).encode())
    
    await nc.subscribe("jobs.captcha.solve", cb=captcha_handler)
    print("✓ Subscrito em jobs.captcha.solve para resolver captchas")

    stop = asyncio.Future()
    await stop

if __name__ == '__main__':
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
