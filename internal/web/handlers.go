package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"golearning/internal/content"
	"golearning/internal/practice"
	"golearning/internal/progress"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server — HTTP-сервер.
type Server struct {
	contentRepo  *content.Repository
	progressRepo *progress.Repository
	checker      *practice.Checker
	templates    *template.Template
}

// NewServer создаёт новый сервер.
func NewServer(contentRepo *content.Repository, progressRepo *progress.Repository, checker *practice.Checker) (*Server, error) {
	// Инициализируем Markdown парсер с подсветкой синтаксиса
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // GitHub Flavored Markdown
			highlighting.NewHighlighting(
				highlighting.WithStyle("monokai"),
			),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // Разрешаем HTML в Markdown
		),
	)

	// Загружаем шаблоны
	funcMap := template.FuncMap{
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"markdown": func(s string) template.HTML {
			var buf bytes.Buffer
			if err := md.Convert([]byte(s), &buf); err != nil {
				return template.HTML("<p>Ошибка рендеринга</p>")
			}
			return template.HTML(buf.String())
		},
		"sectionIcon": func(kind content.SectionKind) string {
			switch kind {
			case content.SectionOverview:
				return "💡"
			case content.SectionTheory:
				return "📖"
			case content.SectionSyntax:
				return "📋"
			case content.SectionExamples:
				return "💻"
			case content.SectionPitfalls:
				return "⚠️"
			case content.SectionLinks:
				return "🔗"
			case content.SectionExtra:
				return "📚"
			default:
				return "📄"
			}
		},
		"statusIcon": func(status progress.Status) string {
			switch status {
			case progress.StatusDone:
				return "✅"
			case progress.StatusReading:
				return "📖"
			default:
				return "⬜"
			}
		},
		"statusClass": func(status progress.Status) string {
			switch status {
			case progress.StatusDone:
				return "status-done"
			case progress.StatusReading:
				return "status-reading"
			default:
				return "status-new"
			}
		},
		"mulf": func(a, b float64) float64 {
			return a * b
		},
		"divf": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		contentRepo:  contentRepo,
		progressRepo: progressRepo,
		checker:      checker,
		templates:    tmpl,
	}, nil
}

// Router возвращает HTTP-роутер.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Статические файлы
	staticSubFS, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS))))

	// HTML страницы
	r.Get("/", s.handleIndex)
	r.Get("/lessons/{slug}", s.handleLesson)
	r.Get("/search", s.handleSearch)
	r.Get("/projects", s.handleProjects)

	// API
	r.Post("/api/progress/lesson/{id}", s.handleUpdateProgress)
	r.Post("/api/progress/reset", s.handleResetProgress)
	r.Post("/api/notes/lesson/{id}", s.handleSaveNote)
	r.Post("/api/run", s.handleRun)
	r.Post("/api/check", s.handleCheck)
	r.Post("/api/tasks/{id}/complete", s.handleCompleteTask)

	return r
}

// --- Page Handlers ---

// handleIndex — главная страница со списком уроков.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Загружаем все курсы
	courses, err := s.contentRepo.ListCourses()
	if err != nil {
		s.serverError(w, err)
		return
	}

	// Структура для модуля с уроками
	type ModuleWithLessons struct {
		Module  content.Module
		Lessons []content.Lesson
	}

	// Структура для курса с модулями
	type CourseWithModules struct {
		Course  content.Course
		Modules []ModuleWithLessons
	}

	var coursesWithModules []CourseWithModules

	for _, course := range courses {
		// Загружаем модули для курса
		modules, err := s.contentRepo.ListModulesByCourseID(course.ID)
		if err != nil {
			s.serverError(w, err)
			return
		}

		var modulesWithLessons []ModuleWithLessons
		for _, m := range modules {
			lessons, err := s.contentRepo.ListLessonsByModuleID(m.ID)
			if err != nil {
				s.serverError(w, err)
				return
			}
			modulesWithLessons = append(modulesWithLessons, ModuleWithLessons{
				Module:  m,
				Lessons: lessons,
			})
		}

		coursesWithModules = append(coursesWithModules, CourseWithModules{
			Course:  course,
			Modules: modulesWithLessons,
		})
	}

	// Загружаем прогресс
	progressMap, _ := s.progressRepo.GetAllProgress()
	stats, _ := s.progressRepo.GetStats()

	data := map[string]interface{}{
		"Courses":  coursesWithModules,
		"Progress": progressMap,
		"Stats":    stats,
	}

	s.render(w, "index.html", data)
}

// handleLesson — страница урока.
func (s *Server) handleLesson(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	lesson, err := s.contentRepo.GetLessonBySlug(slug)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if lesson == nil {
		http.NotFound(w, r)
		return
	}

	// Загружаем прогресс и заметки
	prog, _ := s.progressRepo.GetProgress(lesson.ID)
	note, _ := s.progressRepo.GetNote(lesson.ID)

	// Автоматически отмечаем как "в процессе чтения"
	if prog.Status == progress.StatusNew {
		s.progressRepo.SetStatus(lesson.ID, progress.StatusReading)
		prog.Status = progress.StatusReading
	}

	// Загружаем соседние уроки для навигации
	allLessons, _ := s.contentRepo.ListAllLessons()
	var prevLesson, nextLesson *content.Lesson
	for i, l := range allLessons {
		if l.ID == lesson.ID {
			if i > 0 {
				prevLesson = &allLessons[i-1]
			}
			if i < len(allLessons)-1 {
				nextLesson = &allLessons[i+1]
			}
			break
		}
	}

	// Загружаем статистику для шапки
	stats, _ := s.progressRepo.GetStats()

	// Загружаем список выполненных заданий
	completedTasks := make(map[int64]bool)
	if lesson.Tasks != nil {
		for _, task := range lesson.Tasks {
			if completed, _ := s.progressRepo.IsTaskSolvedSuccessfully(task.ID); completed {
				completedTasks[task.ID] = true
			}
		}
	}

	data := map[string]interface{}{
		"Lesson":         lesson,
		"Progress":       prog,
		"Note":           note,
		"PrevLesson":     prevLesson,
		"NextLesson":     nextLesson,
		"Stats":          stats,
		"CompletedTasks": completedTasks,
	}

	s.render(w, "lesson.html", data)
}

// handleSearch — страница поиска.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	var results []content.SearchResult
	var err error

	if query != "" {
		results, err = s.contentRepo.Search(query, 50)
		if err != nil {
			log.Printf("Search error: %v", err)
			// Не показываем ошибку пользователю, просто пустые результаты
		}
	}

	// Загружаем статистику для шапки
	stats, _ := s.progressRepo.GetStats()

	data := map[string]interface{}{
		"Query":   query,
		"Results": results,
		"Stats":   stats,
	}

	s.render(w, "search.html", data)
}

// --- API Handlers ---

// handleUpdateProgress обновляет прогресс урока.
func (s *Server) handleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.badRequest(w, "Invalid lesson ID")
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(w, "Invalid JSON")
		return
	}

	// Используем SetStatus чтобы не затереть очки
	if err := s.progressRepo.SetStatus(id, progress.Status(req.Status)); err != nil {
		s.serverError(w, err)
		return
	}

	s.jsonResponse(w, map[string]interface{}{"success": true})
}

// handleResetProgress сбрасывает весь прогресс обучения.
func (s *Server) handleResetProgress(w http.ResponseWriter, r *http.Request) {
	if err := s.progressRepo.ResetAllProgress(); err != nil {
		s.serverError(w, err)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "Прогресс успешно сброшен",
	})
}

// handleSaveNote сохраняет заметку.
func (s *Server) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.badRequest(w, "Invalid lesson ID")
		return
	}

	var req struct {
		Note string `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(w, "Invalid JSON")
		return
	}

	if err := s.progressRepo.SaveNote(id, req.Note); err != nil {
		s.serverError(w, err)
		return
	}

	s.jsonResponse(w, map[string]interface{}{"success": true})
}

// handleRun выполняет Go-код.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(w, "Invalid JSON")
		return
	}

	if strings.TrimSpace(req.Code) == "" {
		s.badRequest(w, "Code is empty")
		return
	}

	result, err := s.checker.Run(r.Context(), req.Code)
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.jsonResponse(w, result)
}

// handleCheck проверяет решение задания.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID int64  `json:"task_id"`
		Code   string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(w, "Invalid JSON")
		return
	}

	if req.TaskID == 0 {
		s.badRequest(w, "Task ID is required")
		return
	}

	if strings.TrimSpace(req.Code) == "" {
		s.badRequest(w, "Code is empty")
		return
	}

	result, err := s.checker.Check(r.Context(), req.TaskID, req.Code)
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.jsonResponse(w, result)
}

// handleCompleteTask отмечает manual‑задание выполненным (self-report) и начисляет очки один раз.
func (s *Server) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	taskID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || taskID <= 0 {
		s.badRequest(w, "Invalid task ID")
		return
	}

	task, err := s.contentRepo.GetTaskByID(taskID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if task == nil {
		http.NotFound(w, r)
		return
	}

	if strings.TrimSpace(task.Mode) != "manual" {
		s.badRequest(w, "Task is not manual")
		return
	}

	alreadySolved, err := s.progressRepo.IsTaskSolvedSuccessfully(taskID)
	if err != nil {
		s.serverError(w, err)
		return
	}

	pointsAwarded := 0
	if !alreadySolved {
		// Создаём success-submission (для бейджа «✅ Выполнено» и истории)
		submission := &progress.Submission{
			TaskID: taskID,
			Code:   "[manual]",
			Status: "success",
			Stdout: "",
			Stderr: "",
		}
		if err := s.progressRepo.CreateSubmission(submission); err != nil {
			s.serverError(w, err)
			return
		}

		// Начисляем очки только при первом выполнении
		if err := s.progressRepo.SetPracticeDone(task.LessonID, task.Points); err != nil {
			s.serverError(w, err)
			return
		}

		pointsAwarded = task.Points
	}

	s.jsonResponse(w, map[string]interface{}{
		"success":        true,
		"points_awarded": pointsAwarded,
	})
}

// --- Helpers ---

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	log.Printf("Server error: %v", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (s *Server) badRequest(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusBadRequest)
}
