"""
Captcha Solver - Resolução gratuita de captchas usando OpenCV
Conecta via NATS ao serviço Discovery (Go)
"""
import base64
import io
import json
import logging
import os
from typing import Optional, Tuple

import cv2
import numpy as np
from nats.aio.client import Client as NATS

logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s] %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class CaptchaSolver:
    
    def __init__(self):
        self.nats_url = os.getenv('NATS_URL', 'nats://localhost:4222')
        self.nc = None
        
    async def connect(self):
        try:
            self.nc = NATS()
            await self.nc.connect(self.nats_url)
            logger.info(f" Conectado ao NATS: {self.nats_url}")
            logger.info(" Usando Request-Reply simples (sem JetStream)")
            return True
            
            logger.info(f" Conectado ao NATS: {self.nats_url}")
            logger.info(" Usando Request-Reply simples (sem JetStream)")
            return True
                
        except Exception as e:
            logger.error(f" Erro conectando ao NATS: {e}")
            raise
    
    async def disconnect(self):
        if self.nc:
            await self.nc.close()
            logger.info(" Desconectado do NATS")
    
    def decode_image(self, b64_string: str) -> Optional[np.ndarray]:
        try:
            if ',' in b64_string:
                b64_string = b64_string.split(',')[1]
            
            img_bytes = base64.b64decode(b64_string)
            nparr = np.frombuffer(img_bytes, np.uint8)
            img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
            
            if img is None:
                logger.error(" Falha ao decodificar imagem")
                return None
                
            logger.info(f" Imagem decodificada: {img.shape}")
            return img
            
        except Exception as e:
            logger.error(f" Erro decodificando imagem: {e}")
            return None
    
    def sharpen_image(self, image: np.ndarray) -> np.ndarray:
        kernel = np.array([[-1,-1,-1],
                          [-1, 9,-1],
                          [-1,-1,-1]])
        return cv2.filter2D(image, -1, kernel)
    
    def find_piece_position(
        self, 
        background: np.ndarray, 
        piece: np.ndarray,
        threshold: float = 0.3
    ) -> tuple:
        """
        Encontra a posição X onde a peça se encaixa no background
        usando Template Matching com detecção de bordas
        
        Args:
            background: Imagem do background (cenário com buraco)
            piece: Imagem da peça (quebra-cabeça solto)
            threshold: Limite de confiança (0-1)
            
        Returns:
            Tupla (posição_x, confiança) ou (None, 0.0) se não encontrar
        """
        try:
            logger.info(" Iniciando Template Matching Melhorado...")
            
            # 1. Converte para Grayscale
            bg_gray = cv2.cvtColor(background, cv2.COLOR_BGR2GRAY)
            piece_gray = cv2.cvtColor(piece, cv2.COLOR_BGR2GRAY)
            
            logger.info(f"   Background: {bg_gray.shape}, Piece: {piece_gray.shape}")
            
            # 2. Pré-processamento: CLAHE (melhor que equalizeHist)
            clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8,8))
            bg_gray = clahe.apply(bg_gray)
            piece_gray = clahe.apply(piece_gray)
            
            # 3. Sharpening para destacar detalhes
            bg_gray = self.sharpen_image(bg_gray)
            piece_gray = self.sharpen_image(piece_gray)
            
            # 4. Suavização leve para reduzir ruído
            bg_gray = cv2.GaussianBlur(bg_gray, (3, 3), 0)
            piece_gray = cv2.GaussianBlur(piece_gray, (3, 3), 0)
            
            # 4. Testa múltiplos métodos de template matching
            # Nota: TM_CCORR_NORMED pode dar falsos positivos em bordas
            methods = [
                ('TM_CCOEFF_NORMED', cv2.TM_CCOEFF_NORMED),
                ('TM_SQDIFF_NORMED', cv2.TM_SQDIFF_NORMED),
            ]
            
            best_match = -1
            best_x = 0
            best_method = ""
            
            for method_name, method in methods:
                # Template matching direto (sem edges)
                result = cv2.matchTemplate(bg_gray, piece_gray, method)
                
                if method == cv2.TM_SQDIFF_NORMED:
                    # Para SQDIFF, menor é melhor
                    min_val, max_val, min_loc, max_loc = cv2.minMaxLoc(result)
                    match_val = 1.0 - min_val
                    x_pos = min_loc[0]
                else:
                    # Para outros, maior é melhor
                    min_val, max_val, min_loc, max_loc = cv2.minMaxLoc(result)
                    match_val = max_val
                    x_pos = max_loc[0]
                
                logger.info(f"   {method_name}: score = {match_val:.4f}, x = {x_pos}")
                
                # Ignora resultados impossíveis (muito próximos das bordas)
                if x_pos < 10 or x_pos > (bg_gray.shape[1] - piece_gray.shape[1] - 10):
                    logger.warning(f"       Posição {x_pos} descartada (muito próxima da borda)")
                    continue
                
                if match_val > best_match:
                    best_match = match_val
                    best_x = x_pos
                    best_method = method_name
            
            # 5. Método CANNY EDGES com múltiplos thresholds (mais preciso para puzzles)
            canny_configs = [
                (50, 150),   # Menos agressivo (detecta mais bordas)
                (100, 200),  # Padrão
                (150, 250),  # Mais agressivo (bordas mais fortes)
            ]
            
            canny_best_match = -1
            canny_best_x = 0
            
            for low, high in canny_configs:
                bg_edges = cv2.Canny(bg_gray, low, high)
                piece_edges = cv2.Canny(piece_gray, low, high)
                
                result_edges = cv2.matchTemplate(bg_edges, piece_edges, cv2.TM_CCOEFF_NORMED)
                _, max_val, _, max_loc = cv2.minMaxLoc(result_edges)
                
                # Valida posição
                if max_loc[0] >= 10 and max_loc[0] <= (bg_gray.shape[1] - piece_gray.shape[1] - 10):
                    if max_val > canny_best_match:
                        canny_best_match = max_val
                        canny_best_x = max_loc[0]
                        logger.info(f"   CANNY({low},{high}): score = {max_val:.4f}, x = {max_loc[0]}")
            
            # Usa Canny se for significativamente melhor
            if canny_best_match > 0.15 and (canny_best_match > best_match or best_match < 0.3):
                best_match = canny_best_match
                best_x = canny_best_x
                best_method = "CANNY_EDGES"
                logger.info(f"    CANNY escolhido: score = {canny_best_match:.4f}")
            
            logger.info(f" Melhor método: {best_method} (score: {best_match:.4f})")
            
            if best_match < threshold:
                logger.warning(f"  Match abaixo do threshold: {best_match:.4f} < {threshold}")
            
            logger.info(f" Posição encontrada: X = {best_x}px (confiança: {best_match:.2%})")
            
            return best_x, best_match
            
        except Exception as e:
            logger.error(f" Erro no Template Matching: {e}")
            return None, 0.0
    
    def find_piece_position_multiscale(
        self,
        background: np.ndarray,
        piece: np.ndarray
    ) -> tuple:
        """
        Versão mais robusta que testa múltiplas escalas da peça
        Útil quando o tamanho da peça pode variar
        
        Args:
            background: Imagem do background
            piece: Imagem da peça
            
        Returns:
            Tupla (coordenada_x, confiança)
        """
        try:
            logger.info(" Iniciando Template Matching Multi-escala...")
            
            bg_gray = cv2.cvtColor(background, cv2.COLOR_BGR2GRAY)
            piece_gray = cv2.cvtColor(piece, cv2.COLOR_BGR2GRAY)
            
            # Pré-processamento melhorado
            clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8,8))
            bg_gray = clahe.apply(bg_gray)
            piece_gray = clahe.apply(piece_gray)
            
            # Sharpening
            bg_gray = self.sharpen_image(bg_gray)
            piece_gray = self.sharpen_image(piece_gray)
            
            best_match = -1
            best_x = 0
            best_scale = 1.0
            
            # Testa diferentes escalas (80% a 120%)
            for scale in np.linspace(0.8, 1.2, 15):
                h, w = piece_gray.shape
                new_h, new_w = int(h * scale), int(w * scale)
                
                # Redimensiona a peça
                resized_piece = cv2.resize(piece_gray, (new_w, new_h))
                
                # Verifica se a peça cabe no background
                if resized_piece.shape[0] > bg_gray.shape[0] or \
                   resized_piece.shape[1] > bg_gray.shape[1]:
                    continue
                
                # Template matching direto (melhor que edges para multi-escala)
                result = cv2.matchTemplate(bg_gray, resized_piece, cv2.TM_CCOEFF_NORMED)
                _, max_val, _, max_loc = cv2.minMaxLoc(result)
                
                if max_val > best_match:
                    best_match = max_val
                    best_x = max_loc[0]
                    best_scale = scale
                    
                logger.debug(f"   Escala {scale:.2f}: score = {max_val:.4f}, x = {max_loc[0]}")
            
            logger.info(f" Melhor match: X = {best_x}px (escala: {best_scale:.2f}, score: {best_match:.2%})")
            return best_x, best_match
            
        except Exception as e:
            logger.error(f" Erro no Multi-scale Matching: {e}")
            return None
    
    async def solve_slider(self, bg_b64: str, piece_b64: str) -> dict:
        """
        Resolve um captcha de slider
        
        Args:
            bg_b64: Imagem background em Base64
            piece_b64: Imagem da peça em Base64
            
        Returns:
            Dict com resultado: {'x_offset': int, 'success': bool, 'confidence': float}
        """
        try:
            logger.info("🧩 Resolvendo captcha de slider...")
            
            # Decodifica imagens
            background = self.decode_image(bg_b64)
            piece = self.decode_image(piece_b64)
            
            if background is None or piece is None:
                return {
                    'x_offset': 0,
                    'success': False,
                    'error': 'Falha ao decodificar imagens'
                }
            
            # Encontra posição (usa método simples primeiro)
            result = self.find_piece_position(background, piece, threshold=0.3)
            x_position, confidence = result
            
            # Se confiança muito baixa (<20%), tenta multi-escala
            if confidence < 0.2:
                logger.info("🔄 Confiança baixa. Tentando método multi-escala...")
                x_multi, conf_multi = self.find_piece_position_multiscale(background, piece)
                
                # Usa multi-escala se for melhor E não for 0
                if conf_multi > confidence and x_multi > 0:
                    logger.info(f" Multi-escala melhorou: {conf_multi:.2%} > {confidence:.2%}")
                    x_position = x_multi
                    confidence = conf_multi
                elif x_multi == 0:
                    logger.warning(f"  Multi-escala retornou X=0, mantendo valor anterior: {x_position}px")
            
            if x_position is None:
                return {
                    'x_offset': 0,
                    'success': False,
                    'confidence': 0.0,
                    'error': 'Não foi possível encontrar posição'
                }
            
            return {
                'x_offset': int(x_position),
                'success': True,
                'confidence': float(confidence)
            }
            
        except Exception as e:
            logger.error(f" Erro resolvendo captcha: {e}")
            return {
                'x_offset': 0,
                'success': False,
                'error': str(e)
            }
    
    async def handle_captcha_request(self, msg):
        """
        Handler para mensagens NATS de requisição de captcha
        
        Args:
            msg: Mensagem NATS
        """
        try:
            # Parse payload
            data = json.loads(msg.data.decode())
            logger.info(f"📨 Recebida requisição de captcha")
            
            bg_b64 = data.get('background_b64', '')
            piece_b64 = data.get('piece_b64', '')
            
            if not bg_b64 or not piece_b64:
                logger.error(" Payload incompleto")
                response = {
                    'x_offset': 0,
                    'success': False,
                    'error': 'Payload incompleto'
                }
            else:
                # Resolve o captcha
                response = await self.solve_slider(bg_b64, piece_b64)
            
            # Envia resposta
            response_json = json.dumps(response).encode()
            await msg.respond(response_json)
            
            if response['success']:
                logger.info(f" Resposta enviada: x_offset = {response['x_offset']}px")
            else:
                logger.error(f" Falha: {response.get('error', 'Unknown')}")
                
        except Exception as e:
            logger.error(f" Erro processando requisição: {e}")
            try:
                error_response = json.dumps({
                    'x_offset': 0,
                    'success': False,
                    'error': str(e)
                }).encode()
                await msg.respond(error_response)
            except:
                pass
    
    async def start_listening(self):
        """Inicia o loop de escuta de mensagens NATS"""
        logger.info("👂 Aguardando requisições de captcha...")
        logger.info("   Tópico: jobs.captcha.slider")
        
        # Subscribe ao tópico
        await self.nc.subscribe(
            "jobs.captcha.slider",
            cb=self.handle_captcha_request
        )
        
        # Mantém o serviço rodando
        while True:
            await asyncio.sleep(1)


async def main():
    """Função principal"""
    solver = CaptchaSolver()
    
    try:
        await solver.connect()
        await solver.start_listening()
    except KeyboardInterrupt:
        logger.info("🛑 Interrompido pelo usuário")
    except Exception as e:
        logger.error(f" Erro fatal: {e}")
    finally:
        await solver.disconnect()


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
