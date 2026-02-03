package content

import "time"

// SectionKind — тип секции урока.
type SectionKind string

const (
	SectionOverview SectionKind = "overview"
	SectionTheory   SectionKind = "theory"
	SectionSyntax   SectionKind = "syntax"
	SectionExamples SectionKind = "examples"
	SectionPitfalls SectionKind = "pitfalls"
	SectionLinks    SectionKind = "links"
	SectionExtra    SectionKind = "extra"
)

// Course — руководство/курс (верхний уровень иерархии).
type Course struct {
	ID          int64
	Slug        string
	Title       string
	Description string
	Icon        string
	OrderIndex  int
}

// Module — раздел курса (например, "Основы", "Функции", "Структуры").
type Module struct {
	ID         int64
	CourseID   int64
	Slug       string
	Title      string
	OrderIndex int

	// Связанные данные
	Course *Course
}

// Lesson — урок в модуле.
type Lesson struct {
	ID             int64
	ModuleID       int64
	Slug           string
	Title          string
	OrderIndex     int
	SourceURL      string
	BodyMD         string
	ReadingTimeMin int
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// Связанные данные (заполняются при необходимости)
	Module   *Module
	Sections []Section
	Tasks    []Task
}

// Section — секция урока (overview, syntax, examples и т.д.).
type Section struct {
	ID         int64
	LessonID   int64
	Kind       SectionKind
	Title      string
	BodyMD     string
	OrderIndex int
}

// Task — практическое задание.
type Task struct {
	ID               int64
	LessonID         int64
	Title            string
	PromptMD         string
	Criteria         string // Критерии приёмки
	Hints            string // Подсказки
	StarterCode      string
	TestsGo          string
	ExpectedOutput   string // Ожидаемый вывод программы
	RequiredPatterns string // Паттерны, которые должны быть в коде (разделённые |)
	Mode             string // auto (встроенная проверка) / manual (выполнение в IDE)
	Points           int
	OrderIndex       int
}

// StructuredLesson — структурированный урок после обработки rewriter.
type StructuredLesson struct {
	Title          string
	BodyMD         string
	ReadingTimeMin int
	Sections       []Section
	Tasks          []Task
}

// SearchResult — результат поиска.
type SearchResult struct {
	LessonID int64
	Slug     string
	Title    string
	Snippet  string
	Rank     float64
}
