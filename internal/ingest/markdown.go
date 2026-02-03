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
)

// MarkdownImporter импортирует уроки из Markdown файлов.
type MarkdownImporter struct {
	repo    *content.Repository
	baseDir string
}

// NewMarkdownImporter создаёт новый импортёр.
func NewMarkdownImporter(repo *content.Repository, baseDir string) *MarkdownImporter {
	return &MarkdownImporter{
		repo:    repo,
		baseDir: baseDir,
	}
}

// Import импортирует все уроки из директории.
func (m *MarkdownImporter) Import(ctx context.Context) error {
	log.Printf("Импорт уроков из: %s", m.baseDir)

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

// DirEntry представляет директорию или файл.
type DirEntry struct {
	Name  string
	Title string
	Path  string
	Order int
}

// findGuides находит руководства (верхний уровень директорий).
func (m *MarkdownImporter) findGuides() ([]DirEntry, error) {
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

// findChapters находит главы внутри руководства.
func (m *MarkdownImporter) findChapters(guidePath string) ([]DirEntry, error) {
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

// findLessons находит файлы уроков в главе.
func (m *MarkdownImporter) findLessons(chapterPath string) ([]DirEntry, error) {
	entries, err := os.ReadDir(chapterPath)
	if err != nil {
		return nil, err
	}

	var lessons []DirEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		name := entry.Name()
		order, title := m.parseNumberedName(strings.TrimSuffix(name, ".md"))

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

// parseNumberedName парсит имя вида "01_Название_темы" -> (1, "Название темы")
func (m *MarkdownImporter) parseNumberedName(name string) (int, string) {
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

// importLesson импортирует один урок из Markdown файла.
func (m *MarkdownImporter) importLesson(ctx context.Context, moduleID int64, lessonFile DirEntry) error {
	// Читаем содержимое файла
	data, err := os.ReadFile(lessonFile.Path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	mdContent := string(data)

	// Парсим заголовок
	title := lessonFile.Title
	if h1 := m.extractH1(mdContent); h1 != "" {
		title = h1
	}

	// Создаём slug
	slug := m.slugify(title) + "-" + strconv.Itoa(lessonFile.Order)

	// Оцениваем время чтения (примерно 200 слов в минуту)
	wordCount := len(strings.Fields(mdContent))
	readingTime := wordCount / 200
	if readingTime < 5 {
		readingTime = 5
	}

	// Создаём урок
	lesson := &content.Lesson{
		ModuleID:       moduleID,
		Slug:           slug,
		Title:          title,
		OrderIndex:     lessonFile.Order,
		SourceURL:      "",
		BodyMD:         mdContent,
		ReadingTimeMin: readingTime,
	}

	if err := m.repo.CreateLesson(lesson); err != nil {
		return fmt.Errorf("create lesson: %w", err)
	}
	log.Printf("    📄 Урок: %s (ID=%d, ~%d мин)", title, lesson.ID, readingTime)

	// Удаляем старые секции и задания
	m.repo.DeleteSectionsByLessonID(lesson.ID)
	m.repo.DeleteTasksByLessonID(lesson.ID)

	// Парсим и создаём секции
	sections := m.parseSections(mdContent)
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

	// Парсим и создаём задания
	tasks := m.parseTasks(mdContent)
	for i, task := range tasks {
		t := &content.Task{
			LessonID:         lesson.ID,
			Title:            task.Title,
			PromptMD:         task.Prompt,
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

// ParsedSection представляет распознанную секцию.
type ParsedSection struct {
	Kind  content.SectionKind
	Title string
	Body  string
}

// parseSections парсит секции из Markdown.
func (m *MarkdownImporter) parseSections(md string) []ParsedSection {
	var sections []ParsedSection

	// Регулярка для заголовков второго уровня
	re := regexp.MustCompile(`(?m)^## (.+)$`)
	matches := re.FindAllStringSubmatchIndex(md, -1)

	for i, match := range matches {
		titleStart, titleEnd := match[2], match[3]
		title := md[titleStart:titleEnd]

		// Определяем конец секции
		bodyStart := match[1]
		var bodyEnd int
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		} else {
			bodyEnd = len(md)
		}

		body := strings.TrimSpace(md[bodyStart:bodyEnd])

		// Убираем заголовок из body
		body = strings.TrimPrefix(body, "## "+title)
		body = strings.TrimSpace(body)

		// Определяем тип секции по эмодзи или названию
		kind := m.detectSectionKind(title)

		// Пропускаем секции "Практика" и "Полезные ссылки" — они обрабатываются отдельно
		if kind == "practice" || strings.Contains(strings.ToLower(title), "полезные ссылки") {
			continue
		}

		// Убираем эмодзи из заголовка
		cleanTitle := m.cleanSectionTitle(title)

		sections = append(sections, ParsedSection{
			Kind:  kind,
			Title: cleanTitle,
			Body:  body,
		})
	}

	return sections
}

// detectSectionKind определяет тип секции по заголовку.
func (m *MarkdownImporter) detectSectionKind(title string) content.SectionKind {
	lower := strings.ToLower(title)

	switch {
	case strings.Contains(title, "💡") || strings.Contains(lower, "ключевые идеи"):
		return content.SectionOverview
	case strings.Contains(title, "📋") || strings.Contains(lower, "синтаксис"):
		return content.SectionSyntax
	case strings.Contains(title, "💻") || strings.Contains(lower, "пример"):
		return content.SectionExamples
	case strings.Contains(title, "⚠️") || strings.Contains(lower, "ошибк"):
		return content.SectionPitfalls
	case strings.Contains(title, "📝") || strings.Contains(title, "🏋️") || strings.Contains(lower, "практика") || strings.Contains(lower, "практические задания"):
		return "practice"
	default:
		return content.SectionExtra
	}
}

// cleanSectionTitle убирает эмодзи из заголовка секции.
func (m *MarkdownImporter) cleanSectionTitle(title string) string {
	// Убираем известные эмодзи
	emojis := []string{"💡", "📋", "💻", "⚠️", "📝", "🔗", "📚", "🏋️", "📖"}
	result := title
	for _, emoji := range emojis {
		result = strings.ReplaceAll(result, emoji, "")
	}
	return strings.TrimSpace(result)
}

// ParsedTask представляет распознанное задание.
type ParsedTask struct {
	Title            string
	Prompt           string
	StarterCode      string
	Tests            string
	ExpectedOutput   string
	RequiredPatterns string
	Points           int
}

// parseTasks парсит задания из секции "Практические задания".
func (m *MarkdownImporter) parseTasks(md string) []ParsedTask {
	var tasks []ParsedTask

	// Находим секцию "Практические задания" — ищем от ## 🏋️ Практические задания до следующего ## или конца
	practiceStart := -1
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		// Поддерживаем оба варианта: старый "Практика" и новый "Практические задания"
		if strings.HasPrefix(line, "## ") && (strings.Contains(line, "🏋️") || strings.Contains(strings.ToLower(line), "практические задания") || strings.Contains(strings.ToLower(line), "практика")) {
			practiceStart = i + 1
			break
		}
	}

	if practiceStart < 0 {
		return tasks
	}

	// Находим конец секции "Практика"
	practiceEnd := len(lines)
	for i := practiceStart; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || strings.HasPrefix(lines[i], "---") {
			practiceEnd = i
			break
		}
	}

	practiceContent := strings.Join(lines[practiceStart:practiceEnd], "\n")

	// Находим задания — ищем ### Задание
	taskStarts := []int{}
	taskLines := strings.Split(practiceContent, "\n")
	for i, line := range taskLines {
		if strings.HasPrefix(line, "### ") && strings.Contains(strings.ToLower(line), "задание") {
			taskStarts = append(taskStarts, i)
		}
	}

	for idx, start := range taskStarts {
		// Определяем конец задания
		var end int
		if idx+1 < len(taskStarts) {
			end = taskStarts[idx+1]
		} else {
			end = len(taskLines)
		}

		taskContent := strings.Join(taskLines[start:end], "\n")

		// Извлекаем заголовок
		titleLine := taskLines[start]
		title := strings.TrimPrefix(titleLine, "### ")
		title = strings.TrimSpace(title)

		// Ищем начальный код: **Начальный код:** + блок кода
		starterCode := m.extractStarterCode(taskContent)

		// Ищем ожидаемый вывод: **Ожидаемый вывод:** или > Вывод:
		expectedOutput := m.extractExpectedOutput(taskContent)

		// Ищем требуемые паттерны: **Используйте:** или **Должно быть:**
		requiredPatterns := m.extractRequiredPatterns(taskContent)

		// Ищем баллы: **Баллы:** число
		points := m.extractPoints(taskContent, idx)

		// Ищем решение в <details> (если начальный код не найден)
		if starterCode == "" {
			solutionRe := regexp.MustCompile("(?s)<details>.*?```go\n(.+?)```.*?</details>")
			solutionMatch := solutionRe.FindStringSubmatch(taskContent)
			if len(solutionMatch) >= 2 {
				starterCode = m.generateStarterCode(strings.TrimSpace(solutionMatch[1]))
			}
		}

		// Если всё ещё нет — генерируем базовый
		if starterCode == "" {
			starterCode = m.generateStarterCode("")
		}

		// Формируем описание задания (убираем код и детали)
		prompt := m.extractPrompt(taskContent, title)

		tasks = append(tasks, ParsedTask{
			Title:            title,
			Prompt:           prompt,
			StarterCode:      starterCode,
			Tests:            "",
			ExpectedOutput:   expectedOutput,
			RequiredPatterns: requiredPatterns,
			Points:           points,
		})
	}

	return tasks
}

// generateStarterCode создаёт начальный код на основе решения.
func (m *MarkdownImporter) generateStarterCode(solution string) string {
	if solution == "" {
		return `package main

import "fmt"

func main() {
	// Напишите ваш код здесь
	
}
`
	}

	// Упрощаем решение — оставляем структуру, убираем реализацию
	lines := strings.Split(solution, "\n")
	var result []string

	inFunc := false
	braceCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Сохраняем package и import
		if strings.HasPrefix(trimmed, "package") || strings.HasPrefix(trimmed, "import") {
			result = append(result, line)
			continue
		}

		// Сохраняем пустые строки между package/import
		if trimmed == "" && !inFunc {
			result = append(result, line)
			continue
		}

		// Начало функции main
		if strings.HasPrefix(trimmed, "func main()") {
			result = append(result, line)
			inFunc = true
			braceCount = 1
			result = append(result, "\t// Напишите ваш код здесь")
			result = append(result, "\t")
			continue
		}

		// Внутри функции — считаем скобки
		if inFunc {
			braceCount += strings.Count(line, "{") - strings.Count(line, "}")
			if braceCount <= 0 {
				result = append(result, "}")
				inFunc = false
			}
		}
	}

	return strings.Join(result, "\n")
}

// extractStarterCode извлекает начальный код из текста задания.
func (m *MarkdownImporter) extractStarterCode(taskContent string) string {
	// Ищем паттерн: **Начальный код:** + блок кода
	patterns := []string{
		`(?s)\*\*Начальный код[:\*]*\*\*\s*\n\s*` + "```go\n(.+?)```",
		`(?s)\*\*Начальный код[:\*]*\*\*\s*\n\s*` + "```\n(.+?)```",
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(taskContent); len(match) >= 2 {
			return strings.TrimSpace(match[1])
		}
	}

	return ""
}

// extractPoints извлекает баллы из текста задания.
func (m *MarkdownImporter) extractPoints(taskContent string, idx int) int {
	// Ищем паттерн: **Баллы:** число
	re := regexp.MustCompile(`\*\*Баллы[:\*]*\*\*\s*(\d+)`)
	if match := re.FindStringSubmatch(taskContent); len(match) >= 2 {
		if points, err := strconv.Atoi(match[1]); err == nil {
			return points
		}
	}
	// По умолчанию: 10 + (idx * 5)
	return 10 + (idx * 5)
}

// extractPrompt извлекает описание задания, убирая код и служебные блоки.
func (m *MarkdownImporter) extractPrompt(taskContent, title string) string {
	prompt := taskContent

	// Убираем заголовок
	prompt = strings.TrimPrefix(prompt, "### "+title)

	// Убираем блоки кода (начальный код, ожидаемый вывод)
	codeBlockRe := regexp.MustCompile("(?s)```[^`]*```")
	prompt = codeBlockRe.ReplaceAllString(prompt, "")

	// Убираем <details>
	detailsRe := regexp.MustCompile("(?s)<details>.*?</details>")
	prompt = detailsRe.ReplaceAllString(prompt, "")

	// Убираем служебные строки
	linesToRemove := []string{
		`\*\*Начальный код[:\*]*\*\*`,
		`\*\*Ожидаемый вывод[:\*]*\*\*`,
		`\*\*Баллы[:\*]*\*\*\s*\d+`,
	}
	for _, pattern := range linesToRemove {
		re := regexp.MustCompile(pattern)
		prompt = re.ReplaceAllString(prompt, "")
	}

	// Убираем пустые строки подряд
	for strings.Contains(prompt, "\n\n\n") {
		prompt = strings.ReplaceAll(prompt, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(prompt)
}

// extractExpectedOutput извлекает ожидаемый вывод из текста задания.
func (m *MarkdownImporter) extractExpectedOutput(taskContent string) string {
	// Ищем паттерны вида:
	// **Ожидаемый вывод:**
	// ```
	// Hello, World!
	// ```
	patterns := []string{
		`(?s)\*\*Ожидаемый вывод[:\*]*\*\*\s*\n\s*` + "```" + `[^\n]*\n(.+?)` + "```",
		`(?s)>?\s*Вывод[:\s]*\n\s*` + "```" + `[^\n]*\n(.+?)` + "```",
		`(?s)\*\*Результат[:\*]*\*\*\s*\n\s*` + "```" + `[^\n]*\n(.+?)` + "```",
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(taskContent); len(match) >= 2 {
			return strings.TrimSpace(match[1])
		}
	}

	return ""
}

// extractRequiredPatterns извлекает требуемые паттерны из текста задания.
func (m *MarkdownImporter) extractRequiredPatterns(taskContent string) string {
	// Ищем паттерны вида:
	// **Используйте:** `for`, `if`
	// **Должно быть:** fmt.Println
	patterns := []string{
		`\*\*Используйте[:\*]*\*\*\s*(.+)`,
		`\*\*Должно быть[:\*]*\*\*\s*(.+)`,
		`\*\*Обязательно[:\*]*\*\*\s*(.+)`,
	}

	var allPatterns []string
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(taskContent); len(match) >= 2 {
			// Извлекаем код из backticks
			codeRe := regexp.MustCompile("`([^`]+)`")
			codes := codeRe.FindAllStringSubmatch(match[1], -1)
			for _, c := range codes {
				if len(c) >= 2 {
					allPatterns = append(allPatterns, c[1])
				}
			}
		}
	}

	return strings.Join(allPatterns, "|")
}

// computeExpectedOutput вычисляет ожидаемый вывод из решения.
func (m *MarkdownImporter) computeExpectedOutput(solutionCode string) string {
	// Простой парсинг: ищем fmt.Println("...") и извлекаем строки
	re := regexp.MustCompile(`fmt\.Print(?:ln|f)?\s*\(\s*"([^"]*)"`)
	matches := re.FindAllStringSubmatch(solutionCode, -1)

	var outputs []string
	for _, match := range matches {
		if len(match) >= 2 {
			outputs = append(outputs, match[1])
		}
	}

	if len(outputs) > 0 {
		return strings.Join(outputs, "\n")
	}

	return ""
}

// generateTests создаёт простые тесты для задания.
func (m *MarkdownImporter) generateTests(solution string, taskNum int) string {
	// Базовый тест — просто проверяем, что код компилируется и запускается
	return fmt.Sprintf(`package main

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestTask%d(t *testing.T) {
	cmd := exec.Command("go", "run", "main.go")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Программа завершилась с ошибкой:\n%%s\n%%s", stderr.String(), err)
	}

	output := stdout.String()
	if output == "" {
		t.Log("Программа выполнена успешно (без вывода)")
	} else {
		t.Logf("Вывод программы:\n%%s", output)
	}
}
`, taskNum)
}

// extractH1 извлекает заголовок первого уровня.
func (m *MarkdownImporter) extractH1(md string) string {
	re := regexp.MustCompile(`(?m)^# (.+)$`)
	if match := re.FindStringSubmatch(md); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// slugify преобразует строку в slug.
func (m *MarkdownImporter) slugify(s string) string {
	// Транслитерация кириллицы
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

	// Убираем множественные дефисы
	slug := result.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	slug = strings.ToLower(slug)

	return slug
}
