import asyncio
import nats
import cv2
import json
import os
from nats.errors import TimeoutError
from paddleocr import PaddleOCR


def extrair_texto_seguro(ocr_engine, caminho_arquivo):
    if not os.path.exists(caminho_arquivo):
        print(f"Erro: Arquivo não encontrado: {caminho_arquivo}")
        return ""

    img = cv2.imread(caminho_arquivo)
    if img is None:
        print(f"Erro: CV2 não conseguiu ler a imagem: {caminho_arquivo}")
        return ""

    resultado = ocr_engine.ocr(img, cls=True)

    lista_textos = []

    if resultado is not None and len(resultado) > 0 and resultado[0] is not None:
        for linha in resultado[0]:
            texto_encontrado = linha[1][0]
            lista_textos.append(texto_encontrado)

    return " ".join(lista_textos)


async def main():
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")

    print(f"Conectando ao NATS em {nats_url}...")
    try:
        nc = await nats.connect(nats_url)
        js = nc.jetstream()
        print("Conectado ao NATS JetStream!")
    except Exception as e:
        print(f"Falha fatal na conexão NATS: {e}")
        return

    print("Carregando modelo PaddleOCR (pode demorar um pouco)...")
    ocr = PaddleOCR(use_angle_cls=True, lang='en', show_log=False)
    print("Modelo carregado. Aguardando jobs...")

    sub = await js.pull_subscribe("jobs.analyse", "VISION_WORKER")

    while True:
        try:
            msgs = await sub.fetch(1, timeout=5)

            for msg in msgs:
                try:
                    arquivo = msg.data.decode()
                    print(f"Recebido: {arquivo}")

                    texto_final = extrair_texto_seguro(ocr, arquivo)

                    if not texto_final:
                        print("Nenhum texto detectado ou erro na leitura.")
                    else:
                        print(
                            f"Texto Extraído (Preview): {texto_final[:50]}...")

                    payload = {
                        "source_path": arquivo,
                        "text_content": texto_final
                    }
                    payload_bytes = json.dumps(payload).encode()

                    await js.publish("data.text_extracted", payload_bytes)
                    print("Publicado em data.text_extracted")

                    if os.path.exists(arquivo):
                        os.remove(arquivo)
                        print("Arquivo temporário deletado.")

                    await msg.ack()
                    print("Job concluído e Ack enviado.")

                except Exception as e_process:
                    print(
                        f"Erro processando mensagem específica: {e_process}")

        except TimeoutError:
            continue
        except Exception as e:
            print(f"Erro no loop principal: {e}")
            await asyncio.sleep(1)

if __name__ == '__main__':
    asyncio.run(main())
