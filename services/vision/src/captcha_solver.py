"""
Captcha Solver - Resolução gratuita de captchas usando OpenCV
Conecta via NATS ao serviço Discovery (Go)
"""
import base64
import io
import json
import logging
import os
import asyncio
from typing import Optional, Tuple

import cv2
import numpy as np
import time
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
    
    def calculate_edge_score(self, outer: np.ndarray, inner: np.ndarray, mask: np.ndarray) -> float:
        """
        Calcula score baseado em detecção de bordas Canny
        
        Args:
            outer: Imagem outer em grayscale
            inner: Imagem inner em grayscale
            mask: Máscara circular
            
        Returns:
            Score de 0-200 baseado na correlação das bordas
        """
        # Detecta bordas com Canny
        outer_edges = cv2.Canny(outer, 50, 150)
        inner_edges = cv2.Canny(inner, 50, 150)
        
        # Aplica máscara
        outer_masked = cv2.bitwise_and(outer_edges, outer_edges, mask=mask)
        inner_masked = cv2.bitwise_and(inner_edges, inner_edges, mask=mask)
        
        # Calcula correlação apenas nos pixels da máscara
        mask_pixels = mask > 0
        if np.sum(mask_pixels) == 0:
            return 100.0
        
        outer_vals = outer_masked[mask_pixels].astype(np.float64)
        inner_vals = inner_masked[mask_pixels].astype(np.float64)
        
        # Evita divisão por zero
        if np.std(outer_vals) < 1e-6 or np.std(inner_vals) < 1e-6:
            return 100.0
        
        # Correlação de Pearson
        correlation = np.corrcoef(outer_vals, inner_vals)[0, 1]
        if np.isnan(correlation):
            return 100.0
        
        # Normaliza para 0-200
        return (correlation + 1.0) * 100.0
    
    def calculate_color_score(self, outer: np.ndarray, inner: np.ndarray, mask: np.ndarray) -> float:
        """
        Calcula score baseado em histograma de cores RGB
        
        Args:
            outer: Imagem outer em BGR
            inner: Imagem inner em BGR
            mask: Máscara circular
            
        Returns:
            Score de 0-200 baseado na similaridade de cores
        """
        # Calcula histogramas RGB (8 bins por canal = 512 bins total)
        hist_outer = cv2.calcHist([outer], [0, 1, 2], mask, [8, 8, 8], 
                                   [0, 256, 0, 256, 0, 256])
        hist_inner = cv2.calcHist([inner], [0, 1, 2], mask, [8, 8, 8], 
                                   [0, 256, 0, 256, 0, 256])
        
        # Normaliza
        cv2.normalize(hist_outer, hist_outer, alpha=0, beta=1, norm_type=cv2.NORM_MINMAX)
        cv2.normalize(hist_inner, hist_inner, alpha=0, beta=1, norm_type=cv2.NORM_MINMAX)
        
        # Compara usando correlação (melhor que Bhattacharyya para este caso)
        similarity = cv2.compareHist(hist_outer, hist_inner, cv2.HISTCMP_CORREL)
        
        # Normaliza para 0-200 (correlação vai de -1 a 1)
        return (similarity + 1.0) * 100.0
    
    
    async def solve_rotation(self, outer_b64: str, inner_b64: str) -> dict:
        """
        Resolve captcha de rotação usando Transformada Polar e Correlação de Borda.
        Foca na continuidade dos pixels entre o anel interno e externo.
        Executa em thread separada para não bloquear o loop.
        """
        # Executa processamento pesado em thread separada
        return await asyncio.get_running_loop().run_in_executor(
            None, self._solve_rotation_polar_sync, outer_b64, inner_b64
        )

    def _solve_rotation_polar_sync(self, outer_b64: str, inner_b64: str) -> dict:
        """
        Versão síncrona da resolução polar (para rodar no executor).
        """
        try:
            logger.info("🔄 Resolvendo captcha de rotação (Polar + Disc Interior Match)...")
            
            # 1. Decodifica
            outer = self.decode_image(outer_b64)
            inner = self.decode_image(inner_b64)
            
            if outer is None or inner is None:
                return {'success': False, 'error': 'Image decode failed', 'angle': 0}

            # 2. Gray + CLAHE
            clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8,8))
            outer_gray = clahe.apply(cv2.cvtColor(outer, cv2.COLOR_BGR2GRAY))
            inner_gray = clahe.apply(cv2.cvtColor(inner, cv2.COLOR_BGR2GRAY))

            # 3. Transformada Polar (centros independentes)
            h_out, w_out = outer_gray.shape
            h_in, w_in = inner_gray.shape
            
            max_radius = w_out / 2
            polar_w = int(w_out * 2)
            polar_h = int(max_radius)
            flags = cv2.WARP_POLAR_LINEAR + cv2.WARP_FILL_OUTLIERS
            
            center_out = (w_out / 2, h_out / 2)
            center_in = (w_in / 2, h_in / 2)
            
            polar_outer = cv2.warpPolar(outer_gray, (polar_w, polar_h), center_out, max_radius, flags)
            polar_inner = cv2.warpPolar(inner_gray, (polar_w, polar_h), center_in, max_radius, flags)
            
            boundary_r = int(w_in / 2)
            
            # Diagnóstico
            logger.info(f"   📊 Outer centro={np.mean(polar_outer[:boundary_r, :]):.1f}, anel={np.mean(polar_outer[boundary_r:, :]):.1f}")
            logger.info(f"   📊 Inner disco={np.mean(polar_inner[:boundary_r, :]):.1f}, boundary_r={boundary_r}px")
            
            # 4. Máscara circular para o inner (ignora cantos transparentes→preto)
            # Na imagem polar, os cantos do quadrado ficam em raios > boundary_r
            # e em ângulos diagonais. Criamos máscara baseada em threshold.
            inner_mask = (polar_inner[:boundary_r, :] > 5).astype(np.uint8) * 255
            
            # 5. Comparar INTERIOR DO DISCO (ambos têm conteúdo aqui!)
            # Evita centro (< 15px = artefato polar) e borda (>= boundary_r)
            disc_start = 15
            disc_end = boundary_r - 5
            
            inner_disc = polar_inner[disc_start:disc_end, :]
            outer_disc = polar_outer[disc_start:disc_end, :]
            disc_mask = inner_mask[disc_start:disc_end, :]
            
            # Debug: salva tudo
            debug_dir = "/tmp/captcha_debug"
            os.makedirs(debug_dir, exist_ok=True)
            ts = int(time.time() * 1000)
            cv2.imwrite(f"{debug_dir}/{ts}_outer_raw.png", outer)
            cv2.imwrite(f"{debug_dir}/{ts}_inner_raw.png", inner)
            cv2.imwrite(f"{debug_dir}/{ts}_polar_outer.png", polar_outer)
            cv2.imwrite(f"{debug_dir}/{ts}_polar_inner.png", polar_inner)
            cv2.imwrite(f"{debug_dir}/{ts}_inner_disc.png", inner_disc)
            cv2.imwrite(f"{debug_dir}/{ts}_outer_disc.png", outer_disc)
            cv2.imwrite(f"{debug_dir}/{ts}_disc_mask.png", disc_mask)
            
            logger.info(f"   📊 Disc Inner mean={np.mean(inner_disc):.1f}, Outer mean={np.mean(outer_disc):.1f}")
            logger.info(f"   📊 Mask coverage={np.mean(disc_mask)/255*100:.1f}%")
            
            # ── METHOD 1: Grayscale direto com máscara ──
            crop = 20
            inner_cropped = inner_disc[:, crop:-crop].astype(np.float32)
            mask_cropped = disc_mask[:, crop:-crop]
            search_gray = np.hstack([outer_disc, outer_disc]).astype(np.float32)
            
            res_gray = cv2.matchTemplate(search_gray, inner_cropped, cv2.TM_CCORR_NORMED, mask=mask_cropped)
            _, max_val_gray, _, max_loc_gray = cv2.minMaxLoc(res_gray)
            
            best_x_gray = max_loc_gray[0] - crop
            angle_gray = (best_x_gray / polar_w) * 360.0 % 360
            
            logger.info(f"   🎯 Disc Gray: {angle_gray:.2f}° (Conf: {max_val_gray:.4f})")
            
            # ── METHOD 2: Sobel X com máscara ──
            inner_blur = cv2.GaussianBlur(inner_disc, (3, 3), 0)
            outer_blur = cv2.GaussianBlur(outer_disc, (3, 3), 0)
            
            sobel_inner = cv2.convertScaleAbs(cv2.Sobel(inner_blur, cv2.CV_64F, 1, 0, ksize=5))
            sobel_outer = cv2.convertScaleAbs(cv2.Sobel(outer_blur, cv2.CV_64F, 1, 0, ksize=5))
            
            if np.max(sobel_inner) > 0:
                sobel_inner = cv2.normalize(sobel_inner, None, 0, 255, cv2.NORM_MINMAX)
            if np.max(sobel_outer) > 0:
                sobel_outer = cv2.normalize(sobel_outer, None, 0, 255, cv2.NORM_MINMAX)
            
            cv2.imwrite(f"{debug_dir}/{ts}_sobel_inner.png", sobel_inner)
            cv2.imwrite(f"{debug_dir}/{ts}_sobel_outer.png", sobel_outer)
            
            sobel_inner_cropped = sobel_inner[:, crop:-crop]
            search_sobel = np.hstack([sobel_outer, sobel_outer])
            
            res_sobel = cv2.matchTemplate(search_sobel, sobel_inner_cropped, cv2.TM_CCOEFF_NORMED)
            _, max_val_sobel, _, max_loc_sobel = cv2.minMaxLoc(res_sobel)
            
            best_x_sobel = max_loc_sobel[0] - crop
            angle_sobel = (best_x_sobel / polar_w) * 360.0 % 360
            
            logger.info(f"   🎯 Disc Sobel: {angle_sobel:.2f}° (Conf: {max_val_sobel:.2f})")
            
            # ── Escolhe melhor resultado ──
            if max_val_gray >= max_val_sobel:
                final_angle = angle_gray
                final_conf = max_val_gray
                method = "Disc Gray"
            else:
                final_angle = angle_sobel
                final_conf = max_val_sobel
                method = "Disc Sobel"
            
            logger.info(f"   🏆 Vencedor: {method} → {final_angle:.2f}° (Conf: {final_conf:.4f})")
            
            return {
                'angle': int(final_angle),
                'success': True,
                'confidence': float(final_conf)
            }

        except Exception as e:
            logger.error(f"❌ Erro na resolução Polar: {e}")
            import traceback
            logger.error(traceback.format_exc())
            return {'success': False, 'angle': 0}

    async def solve_slider(self, bg_b64: str, piece_b64: str) -> dict:
        """
        Resolve captcha de slider via Template Matching com Máscara.
        Executa em thread separada.
        """
        return await asyncio.get_running_loop().run_in_executor(
            None, self._solve_slider_sync, bg_b64, piece_b64
        )

    def _solve_slider_sync(self, bg_b64: str, piece_b64: str) -> dict:
        try:
            logger.info("🧩 Resolvendo captcha de slider (Masked Matching)...")
            
            background = self.decode_image(bg_b64)
            piece = self.decode_image(piece_b64)
            
            if background is None or piece is None:
                return {'success': False, 'error': 'Image decode failed'}

            # Separa canais da peça (BGR + Alpha)
            # Re-decodifica para garantir alpha channel
            nparr_piece = np.frombuffer(base64.b64decode(piece_b64), np.uint8)
            piece_with_alpha = cv2.imdecode(nparr_piece, cv2.IMREAD_UNCHANGED)
            
            piece_mask = None
            if piece_with_alpha is not None and piece_with_alpha.shape[2] == 4:
                piece_bgr = piece_with_alpha[:, :, :3]
                piece_mask = piece_with_alpha[:, :, 3]
                logger.info("   mask alpha detectado na peça")
            else:
                piece_bgr = piece
                logger.warning("   nenhum canal alpha detectado na peça")
            
            # Converte para Gray
            bg_gray = cv2.cvtColor(background, cv2.COLOR_BGR2GRAY)
            piece_gray = cv2.cvtColor(piece_bgr, cv2.COLOR_BGR2GRAY)
            
            # Template Matching
            if piece_mask is not None:
                res = cv2.matchTemplate(bg_gray, piece_gray, cv2.TM_CCORR_NORMED, mask=piece_mask)
            else:
                res = cv2.matchTemplate(bg_gray, piece_gray, cv2.TM_CCOEFF_NORMED)
                
            _, max_val, _, max_loc = cv2.minMaxLoc(res)
            best_x = max_loc[0]
            
            logger.info(f"   📍 Posição X encontrada: {best_x} (Confiança: {max_val:.2f})")
            
            return {
                'x': int(best_x),
                'y': int(max_loc[1]),
                'success': True,
                'confidence': float(max_val)
            }
            
        except Exception as e:
            logger.error(f"❌ Erro no slider: {e}")
            return {'success': False, 'x': 0}
    
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
                response = await self.solve_rotation(outer_b64, inner_b64)
                
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
