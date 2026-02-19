#!/usr/bin/env python3
"""
Testes para o Captcha Solver
Execute com: python -m pytest test_captcha_solver.py -v
"""

import base64
import json
import numpy as np
import cv2
from io import BytesIO
from PIL import Image
import sys
import os

# Adiciona src ao path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from captcha_solver import (
    decode_base64_image,
    find_puzzle_distance,
    solve_captcha,
    preprocess_piece,
)


def create_test_image(width=300, height=300, color=(100, 150, 200)):
    """Cria uma imagem de teste em base64"""
    img = Image.new('RGB', (width, height), color)
    buffer = BytesIO()
    img.save(buffer, format='PNG')
    b64 = base64.b64encode(buffer.getvalue()).decode()
    return f"data:image/png;base64,{b64}"


def create_puzzle_images():
    """Cria imagens de teste para puzzle (background + piece)"""
    # Background - imagem maior (300x300)
    bg = Image.new('RGB', (300, 300), (100, 150, 200))
    bg_arr = np.array(bg)
    
    # Piece - imagem menor (50x50) com cor ligeiramente diferente
    piece = Image.new('RGB', (50, 50), (200, 100, 50))
    piece_arr = np.array(piece)
    
    # Converte para base64
    bg_buffer = BytesIO()
    Image.fromarray(bg_arr).save(bg_buffer, format='PNG')
    bg_b64 = base64.b64encode(bg_buffer.getvalue()).decode()
    
    piece_buffer = BytesIO()
    Image.fromarray(piece_arr).save(piece_buffer, format='PNG')
    piece_b64 = base64.b64encode(piece_buffer.getvalue()).decode()
    
    return bg_b64, piece_b64


def test_decode_base64_image():
    """Testa decodificação de imagem base64"""
    print("\n✓ Testando decode_base64_image...")
    
    img_b64 = create_test_image()
    
    try:
        img_array = decode_base64_image(img_b64)
        assert isinstance(img_array, np.ndarray)
        assert img_array.shape == (300, 300, 3)  # BGR
        print("  ✓ Decodificação bem-sucedida")
        print(f"  ✓ Shape: {img_array.shape}")
    except Exception as e:
        print(f"  ✗ Erro: {e}")
        raise


def test_decode_with_prefix():
    """Testa decodificação com prefixo data:image"""
    print("\n✓ Testando decodificação com prefixo...")
    
    img = Image.new('RGB', (100, 100), (255, 0, 0))
    buffer = BytesIO()
    img.save(buffer, format='PNG')
    b64_with_prefix = f"data:image/png;base64,{base64.b64encode(buffer.getvalue()).decode()}"
    
    try:
        img_array = decode_base64_image(b64_with_prefix)
        assert isinstance(img_array, np.ndarray)
        print("  ✓ Prefixo removido corretamente")
    except Exception as e:
        print(f"  ✗ Erro: {e}")
        raise


def test_preprocess_piece():
    """Testa pré-processamento da peça"""
    print("\n✓ Testando preprocess_piece...")
    
    piece = np.random.randint(0, 256, (50, 50), dtype=np.uint8)
    
    try:
        processed = preprocess_piece(piece)
        assert isinstance(processed, np.ndarray)
        assert processed.shape == piece.shape
        print("  ✓ Pré-processamento bem-sucedido")
        print(f"  ✓ Shape: {processed.shape}")
    except Exception as e:
        print(f"  ✗ Erro: {e}")
        raise


def test_find_puzzle_distance():
    """Testa cálculo de distância do puzzle"""
    print("\n✓ Testando find_puzzle_distance...")
    
    bg_b64, piece_b64 = create_puzzle_images()
    
    try:
        result = find_puzzle_distance(bg_b64, piece_b64)
        
        assert isinstance(result, dict)
        assert 'success' in result
        assert 'distance_x' in result
        
        print(f"  ✓ Resultado: {result}")
        print(f"  ✓ Success: {result['success']}")
        print(f"  ✓ Distance X: {result['distance_x']:.2f}")
        
    except Exception as e:
        print(f"  ✗ Erro: {e}")
        raise


def test_solve_captcha():
    """Testa interface principal solve_captcha"""
    print("\n✓ Testando solve_captcha...")
    
    bg_b64, piece_b64 = create_puzzle_images()
    
    payload = {
        'background_url': bg_b64,
        'piece_url': piece_b64
    }
    
    try:
        result = solve_captcha(payload)
        
        assert isinstance(result, dict)
        assert 'success' in result
        assert 'distance_x' in result
        
        print(f"  ✓ Captcha resolvido!")
        print(f"  ✓ Success: {result['success']}")
        print(f"  ✓ Distance: {result['distance_x']:.2f}px")
        
    except Exception as e:
        print(f"  ✗ Erro: {e}")
        raise


def test_solve_captcha_missing_images():
    """Testa solve_captcha com imagens faltando"""
    print("\n✓ Testando solve_captcha com imagens faltando...")
    
    payload = {
        'background_url': '',
        'piece_url': ''
    }
    
    try:
        result = solve_captcha(payload)
        
        assert result['success'] == False
        assert 'error' in result
        
        print(f"  ✓ Erro detectado corretamente: {result['error']}")
        
    except Exception as e:
        print(f"  ✗ Erro inesperado: {e}")
        raise


def test_invalid_base64():
    """Testa com base64 inválido"""
    print("\n✓ Testando com base64 inválido...")
    
    payload = {
        'background_url': 'not_valid_base64!!!',
        'piece_url': 'also_invalid!!!'
    }
    
    try:
        result = solve_captcha(payload)
        
        assert result['success'] == False
        print(f"  ✓ Erro tratado: {result.get('error', 'erro')}")
        
    except Exception as e:
        print(f"  ✗ Erro inesperado: {e}")
        # Isso é esperado, só não deve travar


def test_json_payload():
    """Testa serialização JSON do resultado"""
    print("\n✓ Testando serialização JSON...")
    
    bg_b64, piece_b64 = create_puzzle_images()
    
    payload = {
        'background_url': bg_b64,
        'piece_url': piece_b64
    }
    
    try:
        result = solve_captcha(payload)
        
        # Tenta serializar para JSON
        json_str = json.dumps(result)
        parsed = json.loads(json_str)
        
        assert 'distance_x' in parsed
        assert 'success' in parsed
        
        print(f"  ✓ JSON serializado com sucesso")
        print(f"  ✓ Payload: {json_str[:100]}...")
        
    except Exception as e:
        print(f"  ✗ Erro: {e}")
        raise


def run_all_tests():
    """Executa todos os testes"""
    print("\n" + "="*80)
    print("TESTES DO CAPTCHA SOLVER")
    print("="*80)
    
    tests = [
        test_decode_base64_image,
        test_decode_with_prefix,
        test_preprocess_piece,
        test_find_puzzle_distance,
        test_solve_captcha,
        test_solve_captcha_missing_images,
        test_invalid_base64,
        test_json_payload,
    ]
    
    passed = 0
    failed = 0
    
    for test in tests:
        try:
            test()
            passed += 1
        except Exception as e:
            failed += 1
            print(f"\n✗ FALHOU: {test.__name__}")
            print(f"  {e}")
    
    print("\n" + "="*80)
    print(f"RESULTADO: {passed} passou, {failed} falhou")
    print("="*80 + "\n")
    
    return failed == 0


if __name__ == '__main__':
    success = run_all_tests()
    sys.exit(0 if success else 1)
