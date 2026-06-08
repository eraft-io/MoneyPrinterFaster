# MoneyPrinterFaster

> Escribe una frase, la IA genera vídeos cortos automáticamente. Del guion al vídeo final, sin intervención humana.

**Idiomas:** [English](README.en.md) | [简体中文](README.md) | [繁體中文](README.zh-TW.md) | [Español](README.es.md) | [العربية](README.ar.md) | [Français](README.fr.md)

Pipeline automatizado de producción de vídeos cortos construido con Go. Solo proporciona un tema — el sistema se encarga automáticamente de la generación de guiones con LLM → síntesis de voz TTS → generación de subtítulos → descarga de materiales → composición de audio y vídeo, produciendo finalmente un vídeo MP4 con subtítulos + BGM.

**Demo en línea: [http://47.99.221.247:8080/](http://47.99.221.247:8080/)**

## Arquitectura

```
+---------------------------------------------------------------+
|                     MoneyPrinterFaster                        |
|                                                               |
|   +----------+   +------------+   +------------------------+  |
|   | HTMX Web |   | REST API   |   | WebSocket (Progreso)   |  |
|   | (embed)  |   | (net/http) |   |                        |  |
|   +----+-----+   +-----+------+   +----------+-------------+  |
|        |               |                     |                |
|   +----v---------------v---------------------v------------+   |
|   |                 Router (chi)                            |   |
|   +--------------------------------------------------------+  |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |                Task Dispatcher                         |   |
|   |  - Persistencia SQLite                                 |   |
|   |  - Buffer de canal en memoria                          |   |
|   |  - Prioridad / Cancelar / Reintentar                   |   |
|   +-------------------------------------------------------+   |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |            CPU-Adaptive Worker Pool                    |   |
|   |  - Muestreo periódico de CPU (intervalo 2s)            |   |
|   |  - Ajuste dinámico de workers activos                  |   |
|   |  - Reduce bajo carga, expande en reposo                |   |
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
|   | Cada paso es una interfaz independiente y reemplazable|   |
|   +------------------------------------------------------+   |
|                                                               |
|   +------------------------------------------------------+   |
|   |              External Services                         |   |
|   |  LLM (OpenAI / DeepSeek / Ollama)                     |   |
|   |  TTS (Edge-TTS / Azure / OpenAI)                      |   |
|   |  Material (Pexels / Pixabay / Local / AliyunImage)    |   |
|   |  Subtitle (Edge-TTS / Whisper)                        |   |
|   |  FFmpeg (binario local)                               |   |
|   +------------------------------------------------------+   |
+---------------------------------------------------------------+
```

## WebUI

Interfaz web minimalista en blanco y negro con esquinas rectas, construida con HTMX + Go embed. Cero dependencias de frameworks frontend, despliegue en un solo archivo.

![WebUI](images/webui.png)

## Demo

Los siguientes vídeos son generados completamente por el sistema, sin edición manual:

| Tema | Vídeo de demostración |
|------|-----------------------|
| ¿Por qué la humanidad debe explorar el universo? | http://xhslink.com/o/2hP8Yp1x07g |
| El significado de la lectura | http://xhslink.com/o/3xoZeJm0bUc |

## Estructura del proyecto

```
MoneyPrinterFaster/
├── cmd/server/          # Punto de entrada, inicialización y arranque
├── internal/
│   ├── api/             # Enrutamiento HTTP, handlers, plantillas HTML
│   ├── config/          # Cargador de configuración TOML
│   ├── model/           # Modelos de datos (Task, Params, VideoParams)
│   ├── pipeline/        # Orquestación de pipeline de 5 pasos
│   ├── queue/           # Cola de canal + persistencia SQLite
│   ├── service/         # Integraciones de servicios externos
│   │   ├── compose/     # Composición de vídeo FFmpeg
│   │   ├── llm/         # Generación de guiones LLM (OpenAI/DeepSeek/Ollama)
│   │   ├── material/    # Descarga de materiales (Pexels/AliyunImage/Pixabay)
│   │   ├── subtitle/    # Generación de subtítulos (Edge/Whisper)
│   │   └── tts/         # Síntesis TTS (Edge/Azure/OpenAI)
│   └── worker/          # Worker Pool adaptativo a CPU
├── pkg/utils/           # Utilidades comunes (archivos, detección FFmpeg)
├── web/                 # Recursos estáticos + frontend
├── config.example.toml  # Plantilla de configuración
├── vedios/              # Vídeos de demostración
└── Makefile             # Scripts de compilación
```

## Inicio rápido

### Requisitos

- Go 1.22+
- FFmpeg (con soporte libx264)
- Python 3 + edge-tts (`pip install edge-tts`, se instala automáticamente en la primera ejecución)

### Configuración

```bash
cp config.example.toml config.toml
```

Edita `config.toml` y completa las API Keys:

| Configuración | Descripción | Obligatorio |
|---------------|-------------|-------------|
| `[llm.deepseek].api_key` | API Key de DeepSeek | Sí (al menos un LLM) |
| `[material.pexels].api_keys` | Lista de API Keys de Pexels (búsqueda de vídeos) | Sí (al menos una fuente) |
| `[material.aliyun_image].api_key` | API Key de Alibaba Cloud Bailian (IA texto a imagen) | No (elegir una) |
| `[tts]` | Motor TTS (predeterminado: edge, sin key) | No |

### Ejecución

```bash
# Modo desarrollo
make run

# Compilar
make build
./build/moneyprinterFaster

# Compilación cruzada
make build-darwin
make build-linux
```

Visita `http://localhost:8080` después de iniciar.

### Despliegue con Docker

No es necesario instalar Go / FFmpeg / Python, ejecuta con un solo comando:

```bash
# 1. Preparar configuración
cp config.example.toml config.toml
# Editar config.toml para agregar API Keys

# 2. Construir e iniciar
docker compose up -d
```

O usando Makefile:

```bash
make docker        # Construir imagen
make docker-run    # Iniciar contenedor
make docker-stop   # Detener contenedor
```

O ejecutar directamente con docker:

```bash
docker build -t moneyprinter-faster .
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.toml:/app/config.toml \
  -v mpf-data:/app/data \
  --name moneyprinter \
  --restart unless-stopped \
  moneyprinter-faster
```

**Volúmenes montados:**

| Ruta de montaje | Descripción |
|-----------------|-------------|
| `config.toml:/app/config.toml` | Archivo de configuración, reiniciar contenedor para aplicar cambios |
| `mpf-data:/app/data` | Directorio de datos (SQLite + salida de video), almacenamiento persistente |

> La imagen Docker incluye FFmpeg, edge-tts y fuentes chinas (Noto Sans CJK) listas para usar.

### Flujo de uso

1. Introduce un tema para el vídeo (ej: "El futuro de la inteligencia artificial")
2. Haz clic en "IA Generar Guion" para generar automáticamente, o escribe manualmente
3. Elige fuente de material: **Búsqueda de vídeos** (Pexels) o **Generación de imágenes IA** (Z-Image)
4. Ajusta parámetros (idioma, formato de vídeo, voz, etc.)
5. Haz clic en "Enviar tarea"
6. Monitorea el progreso en la lista de tareas, descarga el MP4 al completar

### Fuentes de material

| Fuente | Descripción | Coste |
|--------|-------------|-------|
| **Búsqueda de vídeos (Pexels)** | Búsqueda automática de clips de vídeo libres de derechos por palabras clave | Gratis |
| **Generación de imágenes IA (Z-Image)** | Texto a imagen de Alibaba Cloud Bailian, genera imágenes IA con efecto zoom Ken Burns | $0.015/imagen |

## Características principales

- **Múltiples fuentes de material**: Búsqueda de vídeos Pexels / Texto a imagen Z-Image de Alibaba Cloud Bailian, seleccionable por tarea
- **Imagen IA a vídeo**: Las fotos generadas por Z-Image se convierten automáticamente en clips de vídeo con efecto zoom Ken Burns
- **Estimación de costes en tiempo real**: Al seleccionar generación de imágenes IA, el frontend estima la cantidad de imágenes y el coste según el guion
- **Worker Pool adaptativo a CPU**: Escala automáticamente según la carga de CPU, previene sobrecarga
- **Cola persistente SQLite**: Las tareas sobreviven reinicios del servicio, soporta consulta de tareas históricas
- **Optimización de memoria FFmpeg**: Normalización por archivo + concatenación con demuxer, evita OOM
- **Estrategia dual de subtítulos**: Prioriza filtro subtitles de libass, fallback a drawtext
- **Control preciso de duración**: ffprobe detecta la duración real, la descarga de materiales no excede el 110% de la duración del audio

## REST API

Ver [docs/api.md](docs/api.md)
