import asyncio
import nats
import cv2
import json
import os
from nats.errors import TimeoutError
from paddleocr import PaddleOCR

def extrair_texto_seguro(ocr_engine, caminho_arquivo):
    if not os.path.exists(caminho_arquivo):
        print(f"Erro: Arquivo nao encontrado: {caminho_arquivo}")
        return ""

    tamanho = os.path.getsize(caminho_arquivo)
    if tamanho == 0:
        print(f"Erro: Arquivo vazio (0 bytes): {caminho_arquivo}")
        return ""

    img = cv2.imread(caminho_arquivo)
    if img is None:
        print(f"Erro: CV2 nao conseguiu ler a imagem (arquivo corrompido?): {caminho_arquivo}")
        return ""

    try:
        resultado = ocr_engine.predict(img)
    except Exception as e:
        print(f"Erro interno no PaddleOCR: {e}")
        return ""

    lista_textos = []

    if resultado:
        for res_item in resultado:
            if 'rec_texts' in res_item:
                textos = res_item['rec_texts']
                if textos and isinstance(textos, list):
                    print(f"Processando {len(textos)} blocos de texto...")
                    for t in textos:
                        if t and isinstance(t, str):
                            t_limpo = t.strip()
                            if len(t_limpo) > 0:
                                lista_textos.append(t_limpo)
    
    return " ".join(lista_textos)


async def main():
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    
    print(f"Conectando ao NATS em {nats_url}...")
    try:
        nc = await nats.connect(nats_url)
        js = nc.jetstream()
        print("Conectado ao NATS JetStream!")
    except Exception as e:
        print(f"Falha fatal na conexao NATS: {e}")
        return

    try:
        print("Verificando infraestrutura (Stream DATA_PIPELINE)...")
        await js.add_stream(name="DATA_PIPELINE", subjects=["data.>"])
        print("Stream DATA_PIPELINE garantida.")
    except Exception as e:
        print(f"Stream ja deve existir: {e}")

    print("Carregando modelo PaddleOCR (pode demorar um pouco)...")
    ocr = PaddleOCR(use_textline_orientation=True, lang='en', enable_mkldnn=False)
    print("Modelo carregado. Aguardando jobs...")

    sub = await js.pull_subscribe("jobs.analyse", "VISION_WORKER", config={
        "durable_name": "VISION_WORKER",
        "ack_wait": 30000000000 
    })

    while True:
        try:
            msgs = await sub.fetch(1, timeout=5)

            for msg in msgs:
                arquivo = ""
                try:
                    arquivo = msg.data.decode()
                    print(f"==========================================")
                    print(f"Recebido Job: {arquivo}")

                    texto_final = extrair_texto_seguro(ocr, arquivo)
                    
                    if not texto_final:
                        print("Aviso: Nenhum texto legivel encontrado na imagem.")
                    else:
                        preview = (texto_final[:75] + '..') if len(texto_final) > 75 else texto_final
                        print(f"Texto Extraido: {preview}")

                        log_path = "/workspaces/Project-Argus/tmp_data/historico_ocr.txt"
                        try:
                            with open(log_path, "a", encoding="utf-8") as f:
                                f.write(f"\n--- Job: {arquivo} ---\n{texto_final}\n")
                            print(f"Texto appendado em {log_path}")
                        except Exception as e_log:
                            print(f"Falha ao salvar log local: {e_log}")

                    payload = {
                        "source_path": arquivo,
                        "text_content": texto_final
                    }
                    payload_bytes = json.dumps(payload).encode()

                    await js.publish("data.text_extracted", payload_bytes)
                    print("Dados enviados para fila 'data.text_extracted'")

                    if os.path.exists(arquivo):
                        os.remove(arquivo)
                        print("Imagem deletada do disco.")

                    await msg.ack()
                    print("Job finalizado com sucesso.")

                except Exception as e_msg:
                    print(f"Erro processando mensagem especifica: {e_msg}")

        except TimeoutError:
            continue
        except Exception as e_main:
            print(f"Erro critico no loop principal: {e_main}")
            await asyncio.sleep(2)

if __name__ == '__main__':
    asyncio.run(main())
