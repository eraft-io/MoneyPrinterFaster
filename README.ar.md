<div dir="rtl">

# MoneyPrinterFaster

> اكتب جملة واحدة، والذكاء الاصطناعي يولّد مقاطع فيديو قصيرة تلقائياً. من النص إلى الفيديو النهائي، بدون تدخل بشري.

**اللغات:** [English](README.en.md) | [简体中文](README.md) | [繁體中文](README.zh-TW.md) | [Español](README.es.md) | [العربية](README.ar.md) | [Français](README.fr.md)

خط إنتاج فيديو قصير آلي مبني بـ Go. فقط قدّم موضوعاً — يتولى النظام تلقائياً: توليد النص بالـ LLM ← تركيب الصوت TTS ← توليد الترجمة ← تنزيل المواد ← تركيب الصوت والفيديو، ويُخرج في النهاية فيديو MP4 مع ترجمة + موسيقى خلفية.

**تجربة مباشرة: [http://47.99.221.247:8080/](http://47.99.221.247:8080/)**

## البنية المعمارية

</div>

```
+---------------------------------------------------------------+
|                     MoneyPrinterFaster                        |
|                                                               |
|   +----------+   +------------+   +------------------------+  |
|   | HTMX Web |   | REST API   |   | WebSocket (Progress)   |  |
|   | (embed)  |   | (net/http) |   |                        |  |
|   +----+-----+   +-----+------+   +----------+-------------+  |
|        |               |                     |                |
|   +----v---------------v---------------------v------------+   |
|   |                 Router (chi)                            |   |
|   +--------------------------------------------------------+  |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |                Task Dispatcher                         |   |
|   |  - SQLite Persistence                                  |   |
|   |  - In-Memory Channel Buffer                            |   |
|   |  - Priority / Cancel / Retry                           |   |
|   +-------------------------------------------------------+   |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |            CPU-Adaptive Worker Pool                    |   |
|   |  - Periodic CPU load sampling (2s interval)            |   |
|   |  - Dynamic active worker adjustment                    |   |
|   |  - Scale down under load, scale up when idle           |   |
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
|   | Each step is an independent, replaceable interface     |   |
|   +------------------------------------------------------+   |
|                                                               |
|   +------------------------------------------------------+   |
|   |              External Services                         |   |
|   |  LLM (OpenAI / DeepSeek / Ollama)                     |   |
|   |  TTS (Edge-TTS / Azure / OpenAI)                      |   |
|   |  Material (Pexels / Pixabay / Local / AliyunImage)    |   |
|   |  Subtitle (Edge-TTS / Whisper)                        |   |
|   |  FFmpeg (Local binary)                                |   |
|   +------------------------------------------------------+   |
+---------------------------------------------------------------+
```

<div dir="rtl">

## واجهة الويب

واجهة ويب بسيطة بألوان أبيض وأسود وزوايا حادة، مبنية بـ HTMX + Go embed. بدون اعتماديات على أطر عمل الواجهة الأمامية، نشر بملف واحد.

![WebUI](images/webui.png)

## عرض توضيحي

المقاطع التالية مُولّدة بالكامل بواسطة النظام دون أي تعديل يدوي:

| الموضوع | فيديو العرض |
|---------|-------------|
| لماذا يجب على البشرية استكشاف الكون | http://xhslink.com/o/2hP8Yp1x07g |
| معنى القراءة | http://xhslink.com/o/3xoZeJm0bUc |

## هيكل المشروع

</div>

```
MoneyPrinterFaster/
├── cmd/server/          # Entry point, initialization & startup
├── internal/
│   ├── api/             # HTTP routing, handlers, HTML templates
│   ├── config/          # TOML config loader
│   ├── model/           # Data models (Task, Params, VideoParams)
│   ├── pipeline/        # 5-step pipeline orchestration
│   ├── queue/           # Channel queue + SQLite persistence
│   ├── service/         # External service integrations
│   │   ├── compose/     # FFmpeg video composition
│   │   ├── llm/         # LLM script generation (OpenAI/DeepSeek/Ollama)
│   │   ├── material/    # Material download (Pexels/AliyunImage/Pixabay)
│   │   ├── subtitle/    # Subtitle generation (Edge/Whisper)
│   │   └── tts/         # TTS synthesis (Edge/Azure/OpenAI)
│   └── worker/          # CPU-adaptive Worker Pool
├── pkg/utils/           # Common utilities (file, FFmpeg path detection)
├── web/                 # Static assets + frontend
├── config.example.toml  # Configuration template
├── vedios/              # Demo videos
└── Makefile             # Build scripts
```

<div dir="rtl">

## البدء السريع

### المتطلبات

- Go 1.22+
- FFmpeg (مع دعم libx264)
- Python 3 + edge-tts (`pip install edge-tts`، يُثبَّت تلقائياً عند أول تشغيل)

### الإعداد

</div>

```bash
cp config.example.toml config.toml
```

<div dir="rtl">

عدّل `config.toml` واملأ مفاتيح API:

| الإعداد | الوصف | مطلوب |
|---------|-------|--------|
| `[llm.deepseek].api_key` | مفتاح DeepSeek API | نعم (LLM واحد على الأقل) |
| `[material.pexels].api_keys` | قائمة مفاتيح Pexels API (البحث عن مقاطع فيديو) | نعم (مصدر واحد على الأقل) |
| `[material.aliyun_image].api_key` | مفتاح Alibaba Cloud Bailian API (نص إلى صورة بالذكاء الاصطناعي) | لا (اختر واحداً) |
| `[tts]` | محرك TTS (الافتراضي: edge، بدون مفتاح) | لا |

### التشغيل

</div>

```bash
# وضع التطوير
make run

# البناء
make build
./build/moneyprinterFaster

# البناء المتقاطع
make build-darwin
make build-linux
```

<div dir="rtl">

زر `http://localhost:8080` بعد البدء.

### النشر باستخدام Docker

لا حاجة لتثبيت Go / FFmpeg / Python، شغّل بأمر واحد:

```bash
# 1. تحضير الإعدادات
cp config.example.toml config.toml
# عدّل config.toml لإضافة مفاتيح API

# 2. بناء وتشغيل
docker compose up -d
```

أو باستخدام Makefile:

```bash
make docker        # بناء الصورة
make docker-run    # تشغيل الحاوية
make docker-stop   # إيقاف الحاوية
```

أو مباشرة مع docker:

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

**نقاط التحميل:**

| مسار التحميل | الوصف |
|--------------|-------|
| `config.toml:/app/config.toml` | ملف الإعدادات، أعد تشغيل الحاوية لتطبيق التغييرات |
| `mpf-data:/app/data` | دليل البيانات (SQLite + مخرجات الفيديو)، تخزين دائم |

> صورة Docker تتضمن FFmpeg و edge-tts والخطوط الصينية (Noto Sans CJK) جاهزة للاستخدام.

### سير العمل

1. أدخل موضوع الفيديو (مثال: "مستقبل الذكاء الاصطناعي")
2. انقر "توليد النص بالذكاء الاصطناعي" للتوليد التلقائي، أو اكتب يدوياً
3. اختر مصدر المواد: **البحث عن فيديو** (Pexels) أو **توليد صور بالذكاء الاصطناعي** (Z-Image)
4. اضبط المعلمات (اللغة، أبعاد الفيديو، الصوت، إلخ)
5. انقر "إرسال المهمة"
6. تابع التقدم في قائمة المهام، وحمّل MP4 عند الانتهاء

### مصادر المواد

| المصدر | الوصف | التكلفة |
|--------|-------|---------|
| **البحث عن فيديو (Pexels)** | بحث تلقائي عن مقاطع فيديو خالية من حقوق الملكية بالكلمات المفتاحية | مجاني |
| **توليد صور بالذكاء الاصطناعي (Z-Image)** | نص إلى صورة من Alibaba Cloud Bailian، يولّد صور ذكاء اصطناعي مع تأثير تكبير Ken Burns | $0.015/صورة |

## الميزات الرئيسية

- **مصادر مواد متعددة**: بحث فيديو Pexels / نص إلى صورة Z-Image من Alibaba Cloud Bailian، قابل للاختيار لكل مهمة
- **تحويل صور الذكاء الاصطناعي إلى فيديو**: الصور المولّدة بواسطة Z-Image تُحوَّل تلقائياً إلى مقاطع فيديو مع تأثير تكبير Ken Burns
- **تقدير التكلفة في الوقت الفعلي**: عند اختيار توليد الصور بالذكاء الاصطناعي، يقدّر الواجهة الأمامية عدد الصور والتكلفة بناءً على النص
- **Worker Pool تكيّفي مع وحدة المعالجة**: يتكيّف تلقائياً حسب حمل المعالج، يمنع التحميل الزائد
- **قائمة انتظار SQLite مستمرة**: المهام تنجو من إعادة تشغيل الخدمة، تدعم الاستعلام عن المهام التاريخية
- **تحسين ذاكرة FFmpeg**: تطبيع لكل ملف + تسلسل بـ demuxer، يتجنّب OOM
- **استراتيجية ترجمة مزدوجة**: يُفضّل مرشح libass subtitles، ويتراجع إلى drawtext
- **تحكم دقيق في المدة**: ffprobe يكتشف المدة الحقيقية، تنزيل المواد لا يتجاوز 110% من مدة الصوت

## REST API

راجع [docs/api.md](docs/api.md)

</div>
