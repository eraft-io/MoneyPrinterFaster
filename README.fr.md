# MoneyPrinterFaster

> Tapez une phrase, l'IA génère automatiquement des vidéos courtes. Du script au montage final, zéro intervention humaine.

**Langues :** [English](README.en.md) | [简体中文](README.md) | [繁體中文](README.zh-TW.md) | [Español](README.es.md) | [العربية](README.ar.md) | [Français](README.fr.md)

Pipeline de production automatisée de vidéos courtes construit avec Go. Fournissez simplement un sujet — le système gère automatiquement la génération de scripts par LLM → synthèse vocale TTS → génération de sous-titres → téléchargement de matériaux → composition audio-vidéo, produisant finalement une vidéo MP4 avec sous-titres + BGM.

**Démo en ligne : [http://47.99.221.247:8080/](http://47.99.221.247:8080/)**

## Architecture

```
+---------------------------------------------------------------+
|                     MoneyPrinterFaster                        |
|                                                               |
|   +----------+   +------------+   +------------------------+  |
|   | HTMX Web |   | REST API   |   | WebSocket (Progression)|  |
|   | (embed)  |   | (net/http) |   |                        |  |
|   +----+-----+   +-----+------+   +----------+-------------+  |
|        |               |                     |                |
|   +----v---------------v---------------------v------------+   |
|   |                 Router (chi)                            |   |
|   +--------------------------------------------------------+  |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |                Task Dispatcher                         |   |
|   |  - Persistance SQLite                                  |   |
|   |  - Tampon canal en mémoire                             |   |
|   |  - Priorité / Annulation / Retry                       |   |
|   +-------------------------------------------------------+   |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |            CPU-Adaptive Worker Pool                    |   |
|   |  - Échantillonnage périodique CPU (intervalle 2s)      |   |
|   |  - Ajustement dynamique des workers actifs             |   |
|   |  - Réduit sous charge, étend au repos                  |   |
|   +----+--------+--------+--------+--------+---------+---+   |
|        |        |        |        |        |         |       |
|   +----v--+ +---v--+ +---v--+ +---v--+ +---v--+ +---v--+    |
|   |Worker| |Worker| |Worker| |Worker| |Worker| |Worker|     |
|   +---+---+ +---+--+ +---+--+ +---+--+ +---+--+ +---+--+    |
|       |         |        |        |        |        |        |
|   +---v---------v--------v--------v--------v--------v----+   |
|   |               Pipeline Engine                         |   |
|   |                                                       |   |
|   | [LLM] -> [TTS] -> [Subtitle] -> [Material] -> [Compose]|  |
|   |                                                       |   |
|   | Chaque étape est une interface indépendante et        |   |
|   | remplaçable                                            |   |
|   +------------------------------------------------------+   |
|                                                               |
|   +------------------------------------------------------+   |
|   |              External Services                         |   |
|   |  LLM (OpenAI / DeepSeek / Ollama)                     |   |
|   |  TTS (Edge-TTS / Azure / OpenAI)                      |   |
|   |  Material (Pexels / Pixabay / Local / AliyunImage)    |   |
|   |  Subtitle (Edge-TTS / Whisper)                        |   |
|   |  FFmpeg (binaire local)                               |   |
|   +------------------------------------------------------+   |
+---------------------------------------------------------------+
```

## WebUI

Interface web minimaliste en noir et blanc aux angles droits, construite avec HTMX + Go embed. Zéro dépendance de framework frontend, déploiement en fichier unique.

![WebUI](images/webui.png)

## Démo

Les vidéos suivantes sont entièrement générées par le système, sans aucun montage manuel :

| Sujet | Vidéo de démonstration |
|-------|------------------------|
| Pourquoi l'humanité doit-elle explorer l'univers | http://xhslink.com/o/2hP8Yp1x07g |
| Le sens de la lecture | http://xhslink.com/o/3xoZeJm0bUc |

## Structure du projet

```
MoneyPrinterFaster/
├── cmd/server/          # Point d'entrée, initialisation et démarrage
├── internal/
│   ├── api/             # Routage HTTP, handlers, templates HTML
│   ├── config/          # Chargeur de configuration TOML
│   ├── model/           # Modèles de données (Task, Params, VideoParams)
│   ├── pipeline/        # Orchestration pipeline en 5 étapes
│   ├── queue/           # File d'attente canal + persistance SQLite
│   ├── service/         # Intégrations de services externes
│   │   ├── compose/     # Composition vidéo FFmpeg
│   │   ├── llm/         # Génération de scripts LLM (OpenAI/DeepSeek/Ollama)
│   │   ├── material/    # Téléchargement de matériaux (Pexels/AliyunImage/Pixabay)
│   │   ├── subtitle/    # Génération de sous-titres (Edge/Whisper)
│   │   └── tts/         # Synthèse TTS (Edge/Azure/OpenAI)
│   └── worker/          # Worker Pool adaptatif au CPU
├── pkg/utils/           # Utilitaires communs (fichiers, détection FFmpeg)
├── web/                 # Ressources statiques + frontend
├── config.example.toml  # Modèle de configuration
├── vedios/              # Vidéos de démonstration
└── Makefile             # Scripts de compilation
```

## Démarrage rapide

### Prérequis

- Go 1.22+
- FFmpeg (avec support libx264)
- Python 3 + edge-tts (`pip install edge-tts`, installé automatiquement au premier lancement)

### Configuration

```bash
cp config.example.toml config.toml
```

Modifiez `config.toml` et renseignez les clés API :

| Configuration | Description | Obligatoire |
|---------------|-------------|-------------|
| `[llm.deepseek].api_key` | Clé API DeepSeek | Oui (au moins un LLM) |
| `[material.pexels].api_keys` | Liste de clés API Pexels (recherche de vidéos) | Oui (au moins une source) |
| `[material.aliyun_image].api_key` | Clé API Alibaba Cloud Bailian (IA texte vers image) | Non (au choix) |
| `[tts]` | Moteur TTS (par défaut : edge, sans clé) | Non |

### Lancement

```bash
# Mode développement
make run

# Compilation
make build
./build/moneyprinterFaster

# Compilation croisée
make build-darwin
make build-linux
```

Visitez `http://localhost:8080` après le démarrage.

### Flux d'utilisation

1. Saisissez un sujet de vidéo (ex : "L'avenir de l'intelligence artificielle")
2. Cliquez sur "IA Générer le script" pour générer automatiquement, ou rédigez manuellement
3. Choisissez la source de matériaux : **Recherche vidéo** (Pexels) ou **Génération d'images IA** (Z-Image)
4. Ajustez les paramètres (langue, format vidéo, voix, etc.)
5. Cliquez sur "Soumettre la tâche"
6. Suivez la progression dans la liste des tâches, téléchargez le MP4 une fois terminé

### Sources de matériaux

| Source | Description | Coût |
|--------|-------------|------|
| **Recherche vidéo (Pexels)** | Recherche automatique de clips vidéo libres de droits par mots-clés | Gratuit |
| **Génération d'images IA (Z-Image)** | Texte vers image d'Alibaba Cloud Bailian, génère des images IA avec effet de zoom Ken Burns | 0,015 $/image |

## Fonctionnalités principales

- **Sources de matériaux multiples** : Recherche vidéo Pexels / Texte vers image Z-Image d'Alibaba Cloud Bailian, sélectionnable par tâche
- **Image IA vers vidéo** : Les photos générées par Z-Image sont automatiquement converties en clips vidéo avec effet de zoom Ken Burns
- **Estimation des coûts en temps réel** : Lors de la sélection de la génération d'images IA, le frontend estime le nombre d'images et le coût en fonction du script
- **Worker Pool adaptatif au CPU** : S'adapte automatiquement à la charge CPU, évite la surcharge
- **File d'attente SQLite persistante** : Les tâches survivent aux redémarrages du service, prise en charge de l'historique des tâches
- **Optimisation mémoire FFmpeg** : Normalisation par fichier + concaténation par demuxer, évite les OOM
- **Stratégie double sous-titres** : Privilégie le filtre subtitles de libass, repli vers drawtext
- **Contrôle précis de la durée** : ffprobe détecte la durée réelle, le téléchargement des matériaux ne dépasse pas 110% de la durée audio

## REST API

Voir [docs/api.md](docs/api.md)
