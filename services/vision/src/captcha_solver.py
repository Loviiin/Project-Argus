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
    """Resolve captchas de slider usando template matching com OpenCV"""
    
    def __init__(self):
        self.nats_url = os.getenv('NATS_URL', 'nats://localhost:4222')
        self.nc = None
        
    async def connect(self):
        """Conecta ao servidor NATS"""
        try:
            self.nc = NATS()
            await self.nc.connect(self.nats_url)
            logger.info(f"✅ Conectado ao NATS: {self.nats_url}")
            logger.info("✅ Usando Request-Reply simples (sem JetStream)")
            return True
            
            logger.info(f"✅ Conectado ao NATS: {self.nats_url}")
            logger.info("✅ Usando Request-Reply simples (sem JetStream)")
            return True
                
        except Exception as e:
            logger.error(f"❌ Erro conectando ao NATS: {e}")
            raise
    
    async def disconnect(self):
        """Desconecta do NATS"""
        if self.nc:
            await self.nc.close()
            logger.info("🔌 Desconectado do NATS")
    
    def decode_image(self, b64_string: str) -> Optional[np.ndarray]:
        """
        Decodifica uma imagem Base64 para array NumPy
        
        Args:
            b64_string: String Base64 da imagem
            
        Returns:
            Array NumPy da imagem ou None se falhar
        """
        try:
            # Remove prefixo data URL se existir
            if ',' in b64_string:
                b64_string = b64_string.split(',')[1]
            
            # Decodifica Base64
            img_bytes = base64.b64decode(b64_string)
            
            # Converte para array NumPy
            nparr = np.frombuffer(img_bytes, np.uint8)
            
            # Decodifica imagem
            img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
            
            if img is None:
                logger.error("❌ Falha ao decodificar imagem")
                return None
                
            logger.info(f"✅ Imagem decodificada: {img.shape}")
            return img
            
        except Exception as e:
            logger.error(f"❌ Erro decodificando imagem: {e}")
            return None
    
    def sharpen_image(self, image: np.ndarray) -> np.ndarray:
        """Aplica sharpening para melhorar detalhes"""
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
            logger.info("🔍 Iniciando Template Matching Melhorado...")
            
            # 1. Converte para Grayscale
            bg_gray = cv2.cvtColor(background, cv2.COLOR_BGR2GRAY)
            piece_gray = cv2.cvtColor(piece, cv2.COLOR_BGR2GRAY)
            
            logger.info(f"   Background: {bg_gray.shape}, Piece: {piece_gray.shape}")
            
            # 2. Pré-processamento: CLAHE (melhor que equalizeHist)
            clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8,8))
            bg_gray = clahe.apply(bg_gray)
            piece_gray = clahe.apply(piece_gray)

            # Margem dinâmica para bordas (evita descartar quando a peça quase ocupa o background)
            max_x_limit = bg_gray.shape[1] - piece_gray.shape[1]
            if max_x_limit <= 0:
                logger.warning("⚠️  Peça maior ou igual ao background. Abortando matching.")
                return None, 0.0

            margin = max(1, int(max_x_limit * 0.05))
            margin = min(margin, 8)
            min_x = 1
            max_x = max_x_limit
            
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
                if x_pos < min_x or x_pos > max_x:
                    logger.warning(f"      ❌ Posição {x_pos} descartada (muito próxima da borda)")
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
                if max_loc[0] >= min_x and max_loc[0] <= max_x:
                    if max_val > canny_best_match:
                        canny_best_match = max_val
                        canny_best_x = max_loc[0]
                        logger.info(f"   CANNY({low},{high}): score = {max_val:.4f}, x = {max_loc[0]}")
            
            # Usa Canny se for significativamente melhor
            if canny_best_match > 0.15 and (canny_best_match > best_match or best_match < 0.3):
                best_match = canny_best_match
                best_x = canny_best_x
                best_method = "CANNY_EDGES"
                logger.info(f"   ✨ CANNY escolhido: score = {canny_best_match:.4f}")
            
            logger.info(f"🏆 Melhor método: {best_method} (score: {best_match:.4f})")

            if best_match < 0 or best_x <= 0:
                logger.warning("⚠️  Nenhum match válido encontrado")
                return None, 0.0
            
            if best_match < threshold:
                logger.warning(f"⚠️  Match abaixo do threshold: {best_match:.4f} < {threshold}")
            
            logger.info(f"✅ Posição encontrada: X = {best_x}px (confiança: {best_match:.2%})")
            
            return best_x, best_match
            
        except Exception as e:
            logger.error(f"❌ Erro no Template Matching: {e}")
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
            logger.info("🔍 Iniciando Template Matching Multi-escala...")
            
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

                max_x_limit = bg_gray.shape[1] - resized_piece.shape[1]
                if max_x_limit <= 0:
                    continue

                margin = max(1, int(max_x_limit * 0.05))
                margin = min(margin, 8)
                min_x = 1
                max_x = max_x_limit

                if max_loc[0] < min_x or max_loc[0] > max_x:
                    continue
                
                if max_val > best_match:
                    best_match = max_val
                    best_x = max_loc[0]
                    best_scale = scale
                    
                logger.debug(f"   Escala {scale:.2f}: score = {max_val:.4f}, x = {max_loc[0]}")
            
            if best_match < 0 or best_x <= 0:
                logger.warning("⚠️  Multi-escala não encontrou match válido")
                return None, 0.0

            logger.info(f"✅ Melhor match: X = {best_x}px (escala: {best_scale:.2f}, score: {best_match:.2%})")
            return best_x, best_match
            
        except Exception as e:
            logger.error(f"❌ Erro no Multi-scale Matching: {e}")
            return None
    
    def solve_rotation(self, outer_b64: str, inner_b64: str) -> dict:
        """
        Resolve um captcha de rotação usando força bruta
        
        O captcha do TikTok mostra uma foto dividida em dois círculos:
        - Outer: círculo externo que gira no sentido ANTI-HORÁRIO
        - Inner: círculo interno que gira no sentido HORÁRIO
        
        Quando o slider move, ambos giram em direções opostas.
        Precisamos encontrar o ângulo do slider onde as imagens se alinham.
        
        Args:
            outer_b64: Imagem do círculo externo em Base64
            inner_b64: Imagem do círculo interno em Base64
            
        Returns:
            Dict com resultado: {'angle': int, 'success': bool, 'confidence': float}
        """
        try:
            logger.info("🔄 Resolvendo captcha de rotação...")
            logger.info("   Outer gira anti-horário, Inner gira horário")
            
            # Decodifica imagens
            outer = self.decode_image(outer_b64)
            inner = self.decode_image(inner_b64)
            
            if outer is None or inner is None:
                return {
                    'angle': 0,
                    'success': False,
                    'confidence': 0.0,
                    'error': 'Falha ao decodificar imagens'
                }
            
            logger.info(f"   Outer: {outer.shape}, Inner: {inner.shape}")
            
            # NÃO converte para grayscale - mantém cores para melhor matching
            # Captchas do TikTok usam imagens coloridas onde cor é importante
            h_outer, w_outer = outer.shape[:2]
            h_inner, w_inner = inner.shape[:2]
            
            # Centro das imagens
            center_outer = (w_outer // 2, h_outer // 2)
            center_inner = (w_inner // 2, h_inner // 2)
            
            # Cria máscara circular para o inner (ignora cantos transparentes)
            inner_mask = np.zeros((h_inner, w_inner), dtype=np.uint8)
            radius_inner = min(w_inner, h_inner) // 2 - 5
            cv2.circle(inner_mask, center_inner, radius_inner, 255, -1)
            
            # Cria máscara de anel para comparação na borda do inner
            # Compara apenas a região próxima à borda onde inner e outer se encontram
            ring_mask = np.zeros((h_inner, w_inner), dtype=np.uint8)
            cv2.circle(ring_mask, center_inner, radius_inner, 255, -1)
            cv2.circle(ring_mask, center_inner, radius_inner - 40, 0, -1)  # Remove mais do centro para focar na borda
            
            # Força bruta: testa de 0 a 360 graus
            # O ângulo de solução vai de 0 a 360 conforme documentação
            best_angle = 0
            best_score = -float('inf')
            step = 2  # Testa a cada 2 graus para mais precisão
            
            scores = []
            
            logger.info(f"🔍 Testando rotações de 0° a 360° (step={step}°)...")
            
            # DEBUG: Salva as imagens originais
            debug_dir = "/tmp/captcha_debug"
            os.makedirs(debug_dir, exist_ok=True)
            cv2.imwrite(f"{debug_dir}/outer_original.png", outer)
            cv2.imwrite(f"{debug_dir}/inner_original.png", inner)
            cv2.imwrite(f"{debug_dir}/ring_mask.png", ring_mask)
            logger.info(f"💾 [Debug] Imagens salvas em {debug_dir}")
            
            for slider_angle in range(0, 361, step):
                # Simula o movimento do slider:
                # - Outer gira slider_angle graus no sentido ANTI-HORÁRIO (positivo no OpenCV)
                # - Inner gira slider_angle graus no sentido HORÁRIO (negativo no OpenCV)
                
                # Rotaciona o outer (anti-horário = ângulo positivo)
                rot_matrix_outer = cv2.getRotationMatrix2D(center_outer, slider_angle, 1.0)
                rotated_outer = cv2.warpAffine(
                    outer, 
                    rot_matrix_outer, 
                    (w_outer, h_outer),
                    flags=cv2.INTER_LINEAR,
                    borderMode=cv2.BORDER_REPLICATE
                )
                
                # Rotaciona o inner (horário = ângulo negativo)
                rot_matrix_inner = cv2.getRotationMatrix2D(center_inner, -slider_angle, 1.0)
                rotated_inner = cv2.warpAffine(
                    inner, 
                    rot_matrix_inner, 
                    (w_inner, h_inner),
                    flags=cv2.INTER_LINEAR,
                    borderMode=cv2.BORDER_REPLICATE
                )
                
                # Extrai a região central do outer que corresponde ao inner
                y_start = (h_outer - h_inner) // 2
                x_start = (w_outer - w_inner) // 2
                outer_center = rotated_outer[y_start:y_start+h_inner, x_start:x_start+w_inner]
                
                # Compara as regiões do anel (borda do inner) - MANTÉM CORES
                # Aplica máscara em cada canal
                outer_masked = cv2.bitwise_and(outer_center, outer_center, mask=ring_mask)
                inner_masked = cv2.bitwise_and(rotated_inner, rotated_inner, mask=ring_mask)
                
                # Calcula diferença em cada canal e média
                diff = cv2.absdiff(outer_masked, inner_masked)
                mean_diff = np.mean(diff[ring_mask > 0]) if np.sum(ring_mask) > 0 else 255
                
                # Inverte para que maior score = melhor match
                score = 255 - mean_diff
                
                scores.append((slider_angle, score))
                
                if score > best_score:
                    best_score = score
                    best_angle = slider_angle
            
            logger.info(f"🏆 Melhor ângulo: {best_angle}° (score: {best_score:.2f})")
            
            # Se há múltiplos ângulos com score similar, escolhe o mais central (próximo de 180°)
            similar_scores = [(angle, score) for angle, score in scores if abs(score - best_score) < 1.0]
            if len(similar_scores) > 1:
                logger.info(f"🔍 Encontrados {len(similar_scores)} ângulos com score similar")
                # Escolhe o mais próximo de 180° (meio da rotação)
                best_candidate = min(similar_scores, key=lambda x: abs(x[0] - 180))
                best_angle = best_candidate[0]
                logger.info(f"✨ Escolhido ângulo mais central: {best_angle}°")
            
            # Refina o resultado: testa ±step graus em passos de 1 grau
            logger.info(f"🔬 Refinando resultado...")
            refined_angle = best_angle
            refined_score = best_score
            
            for slider_angle in range(max(0, best_angle - step - 1), min(361, best_angle + step + 2)):
                rot_matrix_outer = cv2.getRotationMatrix2D(center_outer, slider_angle, 1.0)
                rotated_outer = cv2.warpAffine(
                    outer, rot_matrix_outer, (w_outer, h_outer),
                    flags=cv2.INTER_LINEAR, borderMode=cv2.BORDER_REPLICATE
                )
                
                rot_matrix_inner = cv2.getRotationMatrix2D(center_inner, -slider_angle, 1.0)
                rotated_inner = cv2.warpAffine(
                    inner, rot_matrix_inner, (w_inner, h_inner),
                    flags=cv2.INTER_LINEAR, borderMode=cv2.BORDER_REPLICATE
                )
                
                y_start = (h_outer - h_inner) // 2
                x_start = (w_outer - w_inner) // 2
                outer_center = rotated_outer[y_start:y_start+h_inner, x_start:x_start+w_inner]
                
                outer_masked = cv2.bitwise_and(outer_center, outer_center, mask=ring_mask)
                inner_masked = cv2.bitwise_and(rotated_inner, rotated_inner, mask=ring_mask)
                
                diff = cv2.absdiff(outer_masked, inner_masked)
                mean_diff = np.mean(diff[ring_mask > 0]) if np.sum(ring_mask) > 0 else 255
                score = 255 - mean_diff
                
                if score > refined_score:
                    refined_score = score
                    refined_angle = slider_angle
            
            best_angle = refined_angle
            best_score = refined_score
            logger.info(f"✨ Refinado: {best_angle}° (score: {best_score:.2f})")
            
            # DEBUG: Salva as melhores 3 rotações para análise visual
            logger.info(f"💾 [Debug] Salvando top 3 rotações...")
            scores_sorted = sorted(scores, key=lambda x: x[1], reverse=True)[:3]
            
            for rank, (angle_deg, score_val) in enumerate(scores_sorted, 1):
                # Rotaciona outer
                rot_matrix_outer = cv2.getRotationMatrix2D(center_outer, angle_deg, 1.0)
                rotated_outer = cv2.warpAffine(
                    outer, rot_matrix_outer, (w_outer, h_outer),
                    flags=cv2.INTER_LINEAR, borderMode=cv2.BORDER_REPLICATE
                )
                
                # Rotaciona inner
                rot_matrix_inner = cv2.getRotationMatrix2D(center_inner, -angle_deg, 1.0)
                rotated_inner = cv2.warpAffine(
                    inner, rot_matrix_inner, (w_inner, h_inner),
                    flags=cv2.INTER_LINEAR, borderMode=cv2.BORDER_REPLICATE
                )
                
                # Extrai região central
                y_start = (h_outer - h_inner) // 2
                x_start = (w_outer - w_inner) // 2
                outer_center = rotated_outer[y_start:y_start+h_inner, x_start:x_start+w_inner]
                
                # Salva comparação lado a lado (EM CORES)
                comparison = np.hstack([outer_center, rotated_inner])
                cv2.imwrite(f"{debug_dir}/rank{rank}_angle{int(angle_deg)}_score{score_val:.0f}.png", comparison)
                logger.info(f"   Rank {rank}: {angle_deg}° (score: {score_val:.2f})")
            
            logger.info(f"✅ [Debug] Imagens salvas em {debug_dir}")
            
            # Normaliza confidence (score vai de 0 a 255, queremos 0 a 1)
            confidence = max(0, (best_score - 50) / 205.0)  # Normaliza 50-255 para 0-1
            confidence = min(confidence, 1.0)
            
            # Define sucesso baseado no score e variação
            score_std = float(np.std([s for _, s in scores]))
            
            # SEMPRE retorna sucesso - deixa o Go tentar
            success = True
            
            if best_score < 80:
                logger.warning(f"⚠️  Score baixo: {best_score:.2f}, mas tentando mesmo assim...")
            
            logger.info(f"📊 Stats: score={best_score:.2f}, std={score_std:.2f}, confidence={confidence:.2f}, success={success}")
            
            return {
                'angle': int(best_angle),
                'success': success,
                'confidence': float(confidence)
            }
            
        except Exception as e:
            logger.error(f"❌ Erro resolvendo rotação: {e}")
            import traceback
            traceback.print_exc()
            return {
                'angle': 0,
                'success': False,
                'confidence': 0.0,
                'error': str(e)
            }
    
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
                if conf_multi > confidence and x_multi and x_multi > 0:
                    logger.info(f"✨ Multi-escala melhorou: {conf_multi:.2%} > {confidence:.2%}")
                    x_position = x_multi
                    confidence = conf_multi
                elif x_multi == 0:
                    logger.warning(f"⚠️  Multi-escala retornou X=0, mantendo valor anterior: {x_position}px")
            
            if x_position is None or x_position <= 0:
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
            logger.error(f"❌ Erro resolvendo captcha: {e}")
            return {
                'x_offset': 0,
                'success': False,
                'error': str(e)
            }
    
    async def handle_captcha_request(self, msg):
        """
        Handler para mensagens NATS de requisição de captcha
        Detecta automaticamente o tipo (rotate ou slider)
        
        Args:
            msg: Mensagem NATS
        """
        try:
            # Parse payload
            data = json.loads(msg.data.decode())
            logger.info(f"📨 Recebida requisição de captcha")
            
            # Detecta tipo de captcha
            outer_b64 = data.get('outer_b64', '')
            inner_b64 = data.get('inner_b64', '')
            bg_b64 = data.get('background_b64', '')
            piece_b64 = data.get('piece_b64', '')
            
            # Se tem outer e inner, é rotação
            if outer_b64 and inner_b64:
                logger.info("🔄 Tipo detectado: ROTATE")
                response = self.solve_rotation(outer_b64, inner_b64)
                
                if response['success']:
                    logger.info(f"✅ Resposta enviada: angle = {response['angle']}°")
                else:
                    logger.error(f"❌ Falha: {response.get('error', 'Unknown')}")
            
            # Se tem background e piece, é slider
            elif bg_b64 and piece_b64:
                logger.info("🧩 Tipo detectado: SLIDER")
                response = await self.solve_slider(bg_b64, piece_b64)
                
                if response['success']:
                    logger.info(f"✅ Resposta enviada: x_offset = {response['x_offset']}px")
                else:
                    logger.error(f"❌ Falha: {response.get('error', 'Unknown')}")
            
            else:
                logger.error("❌ Tipo de captcha não reconhecido")
                response = {
                    'success': False,
                    'error': 'Tipo de captcha não reconhecido. Envie (outer_b64+inner_b64) ou (background_b64+piece_b64)'
                }
            
            # Envia resposta
            response_json = json.dumps(response).encode()
            await msg.respond(response_json)
                
        except Exception as e:
            logger.error(f"❌ Erro processando requisição: {e}")
            try:
                error_response = json.dumps({
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
        logger.error(f"❌ Erro fatal: {e}")
    finally:
        await solver.disconnect()


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
