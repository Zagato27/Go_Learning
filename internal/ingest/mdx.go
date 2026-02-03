package ingest

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golearning/internal/content"

	"gopkg.in/yaml.v3"
)

// MDXImporter импортирует уроки из MDX файлов.
type MDXImporter struct {
	repo    *content.Repository
	baseDir string
}

// NewMDXImporter создаёт новый MDX импортёр.
func NewMDXImporter(repo *content.Repository, baseDir string) *MDXImporter {
	return &MDXImporter{
		repo:    repo,
		baseDir: baseDir,
	}
}

// LessonMeta — метаданные урока из тега <Meta>.
type LessonMeta struct {
	Module      string `yaml:"module"`
	Order       int    `yaml:"order"`
	ReadingTime int    `yaml:"reading_time"`
}

// Import импортирует все MDX уроки из директории.
func (m *MDXImporter) Import(ctx context.Context) error {
	log.Printf("MDX Импорт уроков из: %s", m.baseDir)

	// Находим все руководства (верхний уровень)
	guides, err := m.findGuides()
	if err != nil {
		return fmt.Errorf("find guides: %w", err)
	}

	// Иконки для курсов
	courseIcons := map[int]string{
		1: "📘", // Руководство по языку Go
		2: "🌐", // Веб-программирование
		3: "🚀", // Продвинутое программирование
	}

	moduleIndex := 0
	for _, guide := range guides {
		log.Printf("📚 Руководство: %s", guide.Title)

		// Создаём курс для руководства
		icon := courseIcons[guide.Order]
		if icon == "" {
			icon = "📚"
		}
		course := &content.Course{
			Slug:        m.slugify(guide.Title),
			Title:       guide.Title,
			Description: "",
			Icon:        icon,
			OrderIndex:  guide.Order,
		}

		if err := m.repo.CreateCourse(course); err != nil {
			log.Printf("  ⚠️ Ошибка создания курса: %v", err)
			continue
		}
		log.Printf("  📚 Курс: %s (ID=%d)", course.Title, course.ID)

		// Находим главы внутри руководства
		chapters, err := m.findChapters(guide.Path)
		if err != nil {
			log.Printf("  ⚠️ Ошибка поиска глав: %v", err)
			continue
		}

		for _, chapter := range chapters {
			// Создаём модуль для главы
			module := &content.Module{
				CourseID:   course.ID,
				Slug:       m.slugify(chapter.Title),
				Title:      chapter.Title,
				OrderIndex: moduleIndex,
			}

			if err := m.repo.CreateModule(module); err != nil {
				log.Printf("  ⚠️ Ошибка создания модуля: %v", err)
				continue
			}
			log.Printf("  📁 Модуль: %s (ID=%d)", module.Title, module.ID)
			moduleIndex++

			// Находим и импортируем уроки
			lessons, err := m.findLessons(chapter.Path)
			if err != nil {
				log.Printf("    ⚠️ Ошибка поиска уроков: %v", err)
				continue
			}

			for _, lessonFile := range lessons {
				if err := m.importLesson(ctx, module.ID, lessonFile); err != nil {
					log.Printf("    ⚠️ Ошибка импорта урока %s: %v", lessonFile.Name, err)
				}
			}
		}
	}

	return nil
}

// importLesson импортирует один урок из MDX файла.
func (m *MDXImporter) importLesson(ctx context.Context, moduleID int64, lessonFile DirEntry) error {
	data, err := os.ReadFile(lessonFile.Path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	mdxContent := string(data)

	// Парсим заголовок (# Title)
	title := lessonFile.Title
	if h1 := m.extractH1(mdxContent); h1 != "" {
		title = h1
	}

	// Парсим метаданные из <Meta>
	meta := m.parseMeta(mdxContent)

	// Создаём slug
	slug := m.slugify(title) + "-" + strconv.Itoa(lessonFile.Order)

	// Время чтения
	readingTime := meta.ReadingTime
	if readingTime == 0 {
		wordCount := len(strings.Fields(mdxContent))
		readingTime = wordCount / 200
		if readingTime < 5 {
			readingTime = 5
		}
	}

	// Создаём урок
	lesson := &content.Lesson{
		ModuleID:       moduleID,
		Slug:           slug,
		Title:          title,
		OrderIndex:     lessonFile.Order,
		SourceURL:      "",
		BodyMD:         mdxContent,
		ReadingTimeMin: readingTime,
	}

	if err := m.repo.CreateLesson(lesson); err != nil {
		return fmt.Errorf("create lesson: %w", err)
	}
	log.Printf("    📄 Урок: %s (ID=%d, ~%d мин)", title, lesson.ID, readingTime)

	// Удаляем старые секции и задания
	m.repo.DeleteSectionsByLessonID(lesson.ID)
	m.repo.DeleteTasksByLessonID(lesson.ID)

	// Парсим секции из MDX тегов
	sections := m.parseMDXSections(mdxContent)

	// Проверяем, есть ли секция Links
	hasLinks := false
	for _, sec := range sections {
		if sec.Kind == content.SectionLinks {
			hasLinks = true
			break
		}
	}

	// Если нет секции Links, пробуем извлечь из соответствующего markdown файла
	if !hasLinks {
		links := m.extractLinksFromMarkdown(lessonFile.Path)
		if links != "" {
			sections = append(sections, MDXSection{
				Kind:  content.SectionLinks,
				Title: "Полезные ссылки",
				Body:  links,
			})
		}
	}

	for i, sec := range sections {
		section := &content.Section{
			LessonID:   lesson.ID,
			Kind:       sec.Kind,
			Title:      sec.Title,
			BodyMD:     sec.Body,
			OrderIndex: i,
		}
		if err := m.repo.CreateSection(section); err != nil {
			log.Printf("      ⚠️ Ошибка создания секции: %v", err)
		}
	}

	// Парсим задания из MDX тегов
	tasks := m.parseMDXTasks(mdxContent)
	for i, task := range tasks {
		t := &content.Task{
			LessonID:         lesson.ID,
			Title:            task.Title,
			PromptMD:         task.Prompt,
			Criteria:         task.Criteria,
			Hints:            task.Hints,
			StarterCode:      task.StarterCode,
			TestsGo:          task.Tests,
			ExpectedOutput:   task.ExpectedOutput,
			RequiredPatterns: task.RequiredPatterns,
			Points:           task.Points,
			OrderIndex:       i,
		}
		if err := m.repo.CreateTask(t); err != nil {
			log.Printf("      ⚠️ Ошибка создания задания: %v", err)
		}
	}

	if len(tasks) > 0 {
		log.Printf("      ✅ %d заданий создано", len(tasks))
	}

	return nil
}

// parseMeta парсит метаданные из тега <Meta>.
func (m *MDXImporter) parseMeta(mdx string) LessonMeta {
	var meta LessonMeta

	re := regexp.MustCompile(`(?s)<Meta>\s*(.*?)\s*</Meta>`)
	match := re.FindStringSubmatch(mdx)
	if len(match) >= 2 {
		yaml.Unmarshal([]byte(match[1]), &meta)
	}

	return meta
}

// MDXSection — секция из MDX.
type MDXSection struct {
	Kind  content.SectionKind
	Title string
	Body  string
}

// parseMDXSections парсит секции из MDX тегов.
func (m *MDXImporter) parseMDXSections(mdx string) []MDXSection {
	var sections []MDXSection

	// Маппинг тегов на типы секций
	tagMap := map[string]content.SectionKind{
		"Overview": content.SectionOverview,
		"Theory":   content.SectionTheory,
		"Syntax":   content.SectionSyntax,
		"Examples": content.SectionExamples,
		"Pitfalls": content.SectionPitfalls,
		"Links":    content.SectionLinks,
	}

	titleMap := map[string]string{
		"Overview": "Ключевые идеи",
		"Theory":   "Теория",
		"Syntax":   "Синтаксис",
		"Examples": "Примеры кода",
		"Pitfalls": "Частые ошибки",
		"Links":    "Полезные ссылки",
	}

	// Порядок секций
	order := []string{"Overview", "Theory", "Syntax", "Examples", "Pitfalls", "Links"}

	for _, tag := range order {
		re := regexp.MustCompile(`(?s)<` + tag + `>\s*(.*?)\s*</` + tag + `>`)
		match := re.FindStringSubmatch(mdx)
		if len(match) >= 2 {
			body := strings.TrimSpace(match[1])
			if body != "" {
				sections = append(sections, MDXSection{
					Kind:  tagMap[tag],
					Title: titleMap[tag],
					Body:  body,
				})
			}
		}
	}

	return sections
}

// MDXTask — задание из MDX.
type MDXTask struct {
	Title            string
	Prompt           string
	Criteria         string
	Hints            string
	StarterCode      string
	Tests            string
	ExpectedOutput   string
	RequiredPatterns string
	Points           int
}

// parseMDXTasks парсит задания из тегов <Task>.
func (m *MDXImporter) parseMDXTasks(mdx string) []MDXTask {
	var tasks []MDXTask

	// Находим все теги <Task>
	taskRe := regexp.MustCompile(`(?s)<Task\s+([^>]*)>(.*?)</Task>`)
	matches := taskRe.FindAllStringSubmatch(mdx, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		attrs := match[1]
		body := match[2]

		task := MDXTask{
			Points: 10, // default
		}

		// Парсим атрибуты: id="1" points="15"
		attrRe := regexp.MustCompile(`(\w+)="([^"]*)"`)
		attrMatches := attrRe.FindAllStringSubmatch(attrs, -1)
		for _, am := range attrMatches {
			if len(am) >= 3 {
				switch am[1] {
				case "points":
					task.Points, _ = strconv.Atoi(am[2])
				}
			}
		}

		// Парсим внутренние теги
		task.Title = m.extractMDXTag(body, "Title")
		task.Prompt = m.extractMDXTag(body, "Prompt")
		task.Criteria = m.extractMDXTag(body, "Criteria")
		task.Hints = m.extractMDXTag(body, "Hints")
		task.StarterCode = m.extractCodeFromTag(body, "StarterCode")
		task.ExpectedOutput = m.extractMDXTag(body, "ExpectedOutput")
		task.RequiredPatterns = m.extractMDXTag(body, "RequiredPatterns")

		// Автоматически генерируем критерии, если не указаны
		if task.Criteria == "" {
			task.Criteria = m.generateCriteria(task.ExpectedOutput, task.RequiredPatterns)
		}

		// Если StarterCode пустой, генерируем базовый
		if task.StarterCode == "" {
			task.StarterCode = `package main

import "fmt"

func main() {
	// Напишите ваш код здесь
	
}
`
		}

		if task.Title != "" {
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// generateCriteria автоматически генерирует критерии приёмки.
func (m *MDXImporter) generateCriteria(expectedOutput, requiredPatterns string) string {
	var criteria []string

	// Базовый критерий
	criteria = append(criteria, "- Программа компилируется без ошибок")

	// Критерий по выводу
	if expectedOutput != "" {
		criteria = append(criteria, "- Вывод программы точно соответствует ожидаемому результату")
	}

	// Критерий по паттернам
	if requiredPatterns != "" {
		patterns := strings.Split(requiredPatterns, "|")
		if len(patterns) == 1 {
			criteria = append(criteria, fmt.Sprintf("- В коде используется: `%s`", strings.TrimSpace(patterns[0])))
		} else {
			var patternList []string
			for _, p := range patterns {
				patternList = append(patternList, "`"+strings.TrimSpace(p)+"`")
			}
			criteria = append(criteria, fmt.Sprintf("- В коде используются: %s", strings.Join(patternList, ", ")))
		}
	}

	// Дополнительные стандартные критерии
	criteria = append(criteria, "- Код соответствует стандартам Go (gofmt)")

	return strings.Join(criteria, "\n")
}

// extractMDXTag извлекает содержимое тега.
func (m *MDXImporter) extractMDXTag(body, tag string) string {
	re := regexp.MustCompile(`(?s)<` + tag + `>\s*(.*?)\s*</` + tag + `>`)
	match := re.FindStringSubmatch(body)
	if len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// extractCodeFromTag извлекает код из тега (убирает ```go ... ```)
func (m *MDXImporter) extractCodeFromTag(body, tag string) string {
	content := m.extractMDXTag(body, tag)
	if content == "" {
		return ""
	}

	// Убираем ``` обёртку
	codeRe := regexp.MustCompile("(?s)```(?:go)?\\s*\n?(.*?)\\s*```")
	match := codeRe.FindStringSubmatch(content)
	if len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}

	return content
}

// extractH1 извлекает заголовок первого уровня.
func (m *MDXImporter) extractH1(mdx string) string {
	re := regexp.MustCompile(`(?m)^# (.+)$`)
	if match := re.FindStringSubmatch(mdx); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// extractLinksFromMarkdown извлекает секцию "Полезные ссылки" из соответствующего markdown файла.
func (m *MDXImporter) extractLinksFromMarkdown(mdxPath string) string {
	// Преобразуем путь: lessons_mdx -> lessons_ai
	mdPath := strings.Replace(mdxPath, "lessons_mdx", "lessons_ai", 1)
	mdPath = strings.TrimSuffix(mdPath, ".mdx") + ".md"

	data, err := os.ReadFile(mdPath)
	if err != nil {
		return ""
	}

	content := string(data)

	// Ищем секцию "## 🔗 Полезные ссылки" или "## Полезные ссылки"
	linksRe := regexp.MustCompile(`(?s)##\s*(?:🔗\s*)?Полезные ссылки\s*\n(.*?)(?:\n---|\n##|\z)`)
	match := linksRe.FindStringSubmatch(content)
	if len(match) >= 2 {
		links := strings.TrimSpace(match[1])
		if links != "" {
			return links
		}
	}

	return ""
}

// Вспомогательные методы для поиска файлов (аналогичны MarkdownImporter)

func (m *MDXImporter) findGuides() ([]DirEntry, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	var guides []DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Служебные директории/метаданные — не считаем отдельными курсами.
		// Например, lessons_mdx/Проекты содержит ТЗ capstone-проектов для страницы /projects.
		if name == "Проекты" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		order, title := m.parseNumberedName(name)

		guides = append(guides, DirEntry{
			Name:  name,
			Title: title,
			Path:  filepath.Join(m.baseDir, name),
			Order: order,
		})
	}

	sort.Slice(guides, func(i, j int) bool {
		return guides[i].Order < guides[j].Order
	})

	return guides, nil
}

func (m *MDXImporter) findChapters(guidePath string) ([]DirEntry, error) {
	entries, err := os.ReadDir(guidePath)
	if err != nil {
		return nil, err
	}

	var chapters []DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		order, title := m.parseNumberedName(name)

		chapters = append(chapters, DirEntry{
			Name:  name,
			Title: title,
			Path:  filepath.Join(guidePath, name),
			Order: order,
		})
	}

	sort.Slice(chapters, func(i, j int) bool {
		return chapters[i].Order < chapters[j].Order
	})

	return chapters, nil
}

func (m *MDXImporter) findLessons(chapterPath string) ([]DirEntry, error) {
	entries, err := os.ReadDir(chapterPath)
	if err != nil {
		return nil, err
	}

	var lessons []DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Поддерживаем и .md и .mdx
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".mdx") {
			continue
		}

		ext := filepath.Ext(name)
		order, title := m.parseNumberedName(strings.TrimSuffix(name, ext))

		lessons = append(lessons, DirEntry{
			Name:  name,
			Title: title,
			Path:  filepath.Join(chapterPath, name),
			Order: order,
		})
	}

	sort.Slice(lessons, func(i, j int) bool {
		return lessons[i].Order < lessons[j].Order
	})

	return lessons, nil
}

func (m *MDXImporter) parseNumberedName(name string) (int, string) {
	// Паттерн: "01_..." или "Глава_01_..."
	re := regexp.MustCompile(`^(\d+)_(.+)$`)
	if matches := re.FindStringSubmatch(name); len(matches) == 3 {
		order, _ := strconv.Atoi(matches[1])
		title := strings.ReplaceAll(matches[2], "_", " ")
		return order, title
	}

	// Паттерн: "Глава_01_..."
	re2 := regexp.MustCompile(`^Глава_(\d+)_(.+)$`)
	if matches := re2.FindStringSubmatch(name); len(matches) == 3 {
		order, _ := strconv.Atoi(matches[1])
		title := strings.ReplaceAll(matches[2], "_", " ")
		return order, title
	}

	// Без номера
	title := strings.ReplaceAll(name, "_", " ")
	return 0, title
}

func (m *MDXImporter) slugify(s string) string {
	translit := map[rune]string{
		'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
		'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
		'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
		'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
		'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
		'А': "a", 'Б': "b", 'В': "v", 'Г': "g", 'Д': "d", 'Е': "e", 'Ё': "yo",
		'Ж': "zh", 'З': "z", 'И': "i", 'Й': "y", 'К': "k", 'Л': "l", 'М': "m",
		'Н': "n", 'О': "o", 'П': "p", 'Р': "r", 'С': "s", 'Т': "t", 'У': "u",
		'Ф': "f", 'Х': "h", 'Ц': "ts", 'Ч': "ch", 'Ш': "sh", 'Щ': "sch",
		'Ъ': "", 'Ы': "y", 'Ь': "", 'Э': "e", 'Ю': "yu", 'Я': "ya",
	}

	var result strings.Builder
	for _, r := range s {
		if t, ok := translit[r]; ok {
			result.WriteString(t)
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			result.WriteRune('-')
		}
	}

	slug := result.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	slug = strings.ToLower(slug)

	return slug
}
