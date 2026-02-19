"""
collect.py — Ferramenta para anotar samples de captcha coletados durante a execução.

Uso:
    python scripts/collect.py

Fluxo:
    1. Lê amostras de data/raw/<tipo>/
    2. Exibe as imagens para o usuário anotar (ângulo ou offset)
    3. Salva amostra anotada em data/labeled/<tipo>/

TODO: implementar interface de anotação (a definir com tech lead)
"""

# Ponto de partida sugerido:
# - Para rotate: mostrar outer+inner e pedir ângulo em graus
# - Para slider: mostrar bg+piece e pedir offset em pixels

RAW_DIR = "../data/raw"
LABELED_DIR = "../data/labeled"


def annotate_samples():
    raise NotImplementedError("A ser implementado após decisão com tech lead")


if __name__ == "__main__":
    annotate_samples()
