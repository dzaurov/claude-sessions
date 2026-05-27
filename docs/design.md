# ccs — Claude Code Sessions Browser

**Дата:** 2026-05-17
**Автор:** dzaurov
**Статус:** Design

## Проблема

Claude Code хранит все сессии в `~/.claude/projects/<encoded-path>/<uuid>.jsonl`.
Имена сессий — UUID. Когда у пользователя десятки проектов и сотни сессий,
найти нужный чат для `--resume` практически невозможно: нужно помнить, в
какой папке лежит сессия, какой у неё UUID, и нет способа поискать по теме
или дате.

## Цели

1. Найти чат по теме без знания папки проекта (полнотекстовое отображение
   заголовков, fuzzy-поиск).
2. Увидеть последние сессии по всем проектам, отсортированные по дате.
3. Отделить важные чаты от одноразовых экспериментов (pin/hide).
4. Резюмить выбранный чат из **любой** директории, в **том же** терминале,
   с флагом `--dangerously-skip-permissions`.

## Не-цели (v1)

- Удаление .jsonl с диска. Только пометка `hidden` в meta.
- GUI / .app / Spotlight integration. Запуск только из терминала через `ccs`.
- Авто-генерация заголовков через хуки Claude Code. Возможно как v2.
- Синхронизация между машинами, бэкап, экспорт.
- Веб-UI.

## Архитектура

Один Go-бинарь `ccs` с TUI на bubbletea. Состояние хранится в двух местах:

- `~/.claude/projects/**/*.jsonl` — источник истины (управляется Claude Code).
- `~/.claude/cc-sessions/` — собственное состояние:
  - `index.json` — кешированные метаданные сессий (mtime-инвалидация).
  - `meta.json` — пользовательская мета (pinned, tags, hidden, custom_title).
  - `config.toml` — пользовательские настройки.
  - `scan.log` — лог ошибок парсинга.

Бинарь устанавливается симлинком в `~/.local/bin/ccs`. Запуск из любой
директории.

### Структура репозитория

```
cc-sessions/
├── README.md
├── Makefile                  # build, install, test, lint, clean
├── go.mod / go.sum
├── cmd/ccs/main.go           # точка входа
├── internal/
│   ├── index/
│   │   ├── scanner.go        # обход ~/.claude/projects/*/*.jsonl
│   │   ├── parser.go         # стрим JSONL → title, дата, msg_count
│   │   └── cache.go          # ~/.claude/cc-sessions/index.json
│   ├── meta/
│   │   └── store.go          # ~/.claude/cc-sessions/meta.json
│   ├── tui/
│   │   ├── model.go          # bubbletea Model + Update
│   │   ├── view.go           # bubbletea View
│   │   └── keys.go           # KeyMap, хелп
│   ├── launcher/
│   │   └── launcher.go       # syscall.Exec → claude --resume
│   ├── config/
│   │   └── config.go         # ~/.claude/cc-sessions/config.toml
│   └── paths/
│       └── paths.go          # декодирование "-Users-foo-bar" → "/Users/foo/bar"
├── testdata/                 # JSONL-фикстуры
└── test/                     # интеграционные тесты
```

## Компоненты

### 1. Scanner (`internal/index/scanner.go`)

Обходит `~/.claude/projects/*/`. Для каждой проектной папки:

- Декодирует имя в реальный путь через `paths` (см. ниже).
- Список `*.jsonl` файлов (игнорирует папки snapshot'ов с тем же UUID, файлы
  без расширения `.jsonl`, и любые служебные файлы типа `memory/`).
- Для каждого .jsonl запоминает `mtime` и сравнивает с кешем.

Возвращает список `SessionRef{ProjectPath, UUID, Mtime, FilePath}`.

### 2. Parser (`internal/index/parser.go`)

Стримит JSONL построчно через `bufio.Scanner` с увеличенным буфером
(сессии бывают по 50–100 МБ, отдельные строки могут быть очень длинными).

Извлекает:
- `Title` — первое сообщение с `role: "user"`, поле `content` (или
  `message.content` — формат может быть вложенный, см. §"Edge cases"),
  обрезанное до `max_title_length` символов.
- `LastActivity` — timestamp из последней строки (поле `timestamp` если есть,
  иначе mtime файла как fallback).
- `MsgCount` — количество строк.
- `Cwd` — из любого сообщения с полем `cwd` (часто в первом). Это рабочая
  директория, в которую chdir-нем перед `claude --resume`. Если поле есть и
  не равно декодированному project_path, **используем `cwd` из jsonl** —
  Claude Code иногда мигрирует/перемещает.

Парсер останавливается на первом user-сообщении для title, но дочитывает
файл до конца для msg_count и last activity. Для огромных файлов это
дёшево — мы не парсим каждую строку, только сканируем.

### 3. Cache (`internal/index/cache.go`)

Файл `~/.claude/cc-sessions/index.json`:

```json
{
  "version": 1,
  "entries": {
    "/abs/project/path::uuid": {
      "uuid": "...",
      "project_path": "/abs/project/path",
      "title": "...",
      "last_activity": "2026-05-15T14:23:00Z",
      "mtime": "2026-05-15T14:23:00Z",
      "msg_count": 42,
      "cwd": "/abs/project/path"
    }
  }
}
```

Ключ = `project_path + "::" + uuid` — защищает от теоретических коллизий
UUID между проектами.

Инвалидация: при сканировании, если `mtime` на диске > `mtime` в кеше →
перепарсить. Если файл исчез — удалить из кеша.

Атомарная запись: tmp + rename.

### 4. Meta store (`internal/meta/store.go`)

Файл `~/.claude/cc-sessions/meta.json`:

```json
{
  "version": 1,
  "entries": {
    "/abs/project/path::uuid": {
      "pinned": true,
      "tags": ["backend","perf"],
      "hidden": false,
      "custom_title": "...",
      "notes": "..."
    }
  }
}
```

Ключ синхронизирован с cache по формату.

Атомарная запись (tmp + rename) + `flock` на запись, чтобы две запущенные
`ccs` не затёрли друг другу мету.

### 5. TUI (`internal/tui/`)

bubbletea Model:

```go
type Model struct {
    sessions []Session        // отфильтрованные по поиску
    all      []Session        // все, после применения hidden-фильтра
    cursor   int
    search   textinput.Model
    showHidden bool
    err      error
    help     help.Model
    keys     KeyMap
}
```

Layout:

```
┌─ cc-sessions ─────────────────────────────────────────────────┐
│ /jenkins_                                          42 sessions │
├───────────────────────────────────────────────────────────────┤
│ [★] 2026-05-15 14:23  auth-service     fix JWT refresh race │
│     2026-05-12 09:01  notes-cli        rewrite tag storage  │
│ [★] 2026-05-10 18:45  api-gateway      CORS fix /api/track  │
│ ...                                                            │
├───────────────────────────────────────────────────────────────┤
│ ↑↓ nav · ⏎ resume · / search · p pin · h hide · r rescan · ? │
└───────────────────────────────────────────────────────────────┘
```

Хоткеи:

| Клавиша | Действие |
|---------|----------|
| `↑`/`↓` или `k`/`j` | Навигация |
| `Enter` | Резюм выбранной сессии |
| `/` | Войти в режим поиска |
| `Esc` | Выйти из режима поиска / сбросить фильтр |
| `p` | Pin/unpin |
| `h` | Hide/unhide |
| `t` | Toggle: показывать hidden |
| `r` | Принудительный rescan (игнорирует кеш) |
| `?` | Help |
| `q` / `Ctrl-C` | Quit |

Сортировка по умолчанию: pinned первые → по `last_activity` убыв.

Fuzzy-поиск: используем `github.com/sahilm/fuzzy` или `lithammer/fuzzysearch`.
Поиск идёт по конкатенации `custom_title || title + project_path + tags`.

### 6. Launcher (`internal/launcher/launcher.go`)

На Enter:

```go
func Resume(s Session, mode string) error {
    if err := os.Chdir(s.Cwd); err != nil { return err }
    bin, err := exec.LookPath("claude")
    if err != nil { return err }
    args := []string{"claude", "--resume", s.UUID}
    switch mode {
    case "dangerously-skip":
        args = append(args, "--dangerously-skip-permissions")
    case "accept-edits":
        args = append(args, "--permission-mode", "acceptEdits")
    }
    return syscall.Exec(bin, args, os.Environ())
}
```

`syscall.Exec` заменяет текущий процесс — терминал бесшовно переходит из TUI в Claude Code.

### 7. Config (`internal/config/config.go`)

Файл `~/.claude/cc-sessions/config.toml`:

```toml
permission_mode = "dangerously-skip"   # "dangerously-skip" | "accept-edits" | "default"
show_hidden = false
max_title_length = 80
date_format = "2006-01-02 15:04"
```

Если файла нет — создаётся с дефолтами при первом запуске.

### 8. Paths (`internal/paths/paths.go`)

Декодирует имя папки в реальный путь:

- `-Users-alice-Documents-react-dashboard-app` → `/Users/alice/Documents/react/dashboard/app`
- Каждое `-` после первого = `/` ровно по позициям. Простой `strings.ReplaceAll("-", "/")` с
  префиксом `/` — рабочая эвристика, поскольку реальные дефисы в путях в этой системе
  встречаются (например `react-dashboard-app`). Используем `cwd` из самой jsonl как
  достоверный источник, а декодирование папки — только как fallback и для отображения.

Это критичное место — см. §"Edge cases".

## Data flow

```
ccs запуск
  ↓
Load config (~/.claude/cc-sessions/config.toml)
  ↓
Load cache (index.json) + meta (meta.json)
  ↓
Scanner: обходит ~/.claude/projects/*/*.jsonl
  ↓ для каждого файла:
      mtime <= cached.mtime? → берём из кеша
      иначе → Parser извлекает title/activity/count/cwd
  ↓
Сохраняем обновлённый cache (атомарно)
  ↓
Применяем meta (hidden фильтруем, pinned выше, custom_title заменяет title)
  ↓
TUI рендерится
  ↓
Loop: search/nav/pin/hide/...
  ↓
Enter →
  Persist any meta changes
  Chdir(session.Cwd)
  syscall.Exec("claude", ["claude", "--resume", uuid, "--dangerously-skip-permissions"])
  ↓
Process replaced. TUI исчезает, Claude Code запускается в том же терминале.
```

## Edge cases

| Кейс | Поведение |
|------|-----------|
| .jsonl пустой | Skip, лог в `scan.log` |
| .jsonl без user-сообщений | `title = "(no user message)"` |
| Битый JSON в строке | Skip строку, продолжаем; если всё битое — title = "(parse error)" |
| Очень длинная сессия (>100MB) | Стрим-парсинг, не падаем; буфер scanner = 16MB на строку |
| Папка проекта удалена с диска | Сессия отмечается `(missing)`, Enter блокируется, доступны hide/unhide |
| `cwd` из jsonl и декодированный путь расходятся | Доверяем `cwd` |
| `cwd` отсутствует в jsonl вообще | Используем декодированный путь папки |
| `claude` не в PATH | Понятная ошибка с инструкцией |
| Кеш битый | Удаляем, пересканируем с нуля |
| Несколько ccs запущены одновременно | flock на meta.json при записи; кеш — last-writer-wins, не критично |
| UUID совпадает в двух проектах | Ключи в cache/meta включают project_path, конфликта нет |
| Изменение пути проекта (юзер переименовал папку) | Имя папки в `~/.claude/projects/` остаётся со старым кодированием. Используем `cwd` из jsonl как актуальный путь. Если папки больше не существует — `(missing)`. |
| TUI ресайз окна | bubbletea handles, перерисовываем |

## Тестирование

### Unit

- **`parser_test.go`** — фикстуры в `testdata/`:
  - `normal.jsonl` — обычная сессия, 5 сообщений
  - `empty.jsonl` — пустой файл
  - `no_user.jsonl` — только system messages
  - `huge_line.jsonl` — одна строка 20MB (стресс-тест буфера)
  - `partial_corrupt.jsonl` — несколько строк битые, остальные ок
  - `nested_content.jsonl` — content как массив объектов (формат claude code)
- **`scanner_test.go`** — моковая ФС (afero), проверка инкрементальной
  индексации по mtime.
- **`cache_test.go`** — round-trip JSON, атомарная запись, восстановление
  после битого кеша.
- **`meta_test.go`** — атомарная запись, конкурентный доступ (горутины +
  flock), миграция схемы.
- **`paths_test.go`** — декодирование путей, кейсы с дефисами в реальных
  путях.

### TUI

- **Snapshot-тесты** через `github.com/charmbracelet/x/exp/teatest`:
  список сессий, фильтрация, нажатия клавиш, переходы.

### Integration

- Скрытый флаг `ccs --list-json` — выдаёт массив сессий в JSON и завершается
  (без TUI). Используется в интеграционных тестах: создаём временное
  `~/.claude/projects/`, наполняем фикстурами, запускаем `ccs --list-json`,
  проверяем содержимое.
- Тест на Launcher: подменяем `claude` на эхо-скрипт, проверяем, что
  `--resume <uuid> --dangerously-skip-permissions` собирается правильно и
  chdir отрабатывает. `syscall.Exec` нельзя протестировать напрямую (он
  заменяет процесс), но можно протестировать функцию построения команды
  отдельно.

### Ручное тестирование (после имплементации)

- Запустить `ccs` на реальном `~/.claude/projects/` (12 проектов, ~150 сессий)
- Проверить: появляется список с осмысленными заголовками
- Поиск работает по теме (например "jenkins" должен найти соответствующие)
- Pin / hide / unhide → сохраняется в meta, при перезапуске виден
- Enter → реально открывается Claude Code в правильной директории с
  правильным UUID, флаг применён
- Resize терминала, длинные заголовки, пустой список — UI не ломается

## Установка

Репозиторий уже инициализирован в `~/Documents/cc-sessions`. После сборки:

```bash
cd ~/Documents/cc-sessions
make build       # → ./bin/ccs
make install     # → ln -sf $(pwd)/bin/ccs ~/.local/bin/ccs
```

`make install` проверяет, что `~/.local/bin` в PATH; если нет — печатает
инструкцию для `.zshrc`.

## Зависимости

- Go 1.22+
- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles` (textinput, help)
- `github.com/charmbracelet/lipgloss` (стили)
- `github.com/sahilm/fuzzy` (поиск)
- `github.com/BurntSushi/toml` (config)
- `github.com/gofrs/flock` (file locking)
- `github.com/spf13/afero` (моковая ФС для тестов)
- `github.com/charmbracelet/x/exp/teatest` (TUI тесты)

Системные: установленный `claude` в PATH.

## Открытые вопросы

Нет. Все решения зафиксированы в этом доке. Если что-то всплывёт в имплементации —
обновляем док и продолжаем.
