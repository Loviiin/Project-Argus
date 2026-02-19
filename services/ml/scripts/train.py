"""
train.py — Script de treinamento do modelo de resolução de captcha.

Uso:
    python scripts/train.py

Entrada:
    data/labeled/<tipo>/ — pares imagem + label anotados

Saída:
    models/<tipo>_v<versao>.pt — checkpoint do modelo treinado

TODO: implementar após decisão de arquitetura com tech lead
"""

LABELED_DIR = "../data/labeled"
MODELS_DIR = "../models"


def train():
    raise NotImplementedError("A ser implementado após decisão com tech lead")


if __name__ == "__main__":
    train()
