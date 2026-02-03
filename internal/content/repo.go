package content

import (
	"database/sql"
	"fmt"
	"strings"
)

// Repository — репозиторий для работы с контентом.
type Repository struct {
	db *sql.DB
}

// NewRepository создаёт новый репозиторий.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// --- Courses ---

// CreateCourse создаёт или обновляет курс.
func (r *Repository) CreateCourse(c *Course) error {
	_, err := r.db.Exec(
		`INSERT INTO courses (slug, title, description, icon, order_index) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET title = excluded.title, description = excluded.description, 
		 icon = excluded.icon, order_index = excluded.order_index`,
		c.Slug, c.Title, c.Description, c.Icon, c.OrderIndex,
	)
	if err != nil {
		return fmt.Errorf("insert course: %w", err)
	}

	err = r.db.QueryRow("SELECT id FROM courses WHERE slug = ?", c.Slug).Scan(&c.ID)
	if err != nil {
		return fmt.Errorf("get course id: %w", err)
	}

	return nil
}

// GetCourseBySlug возвращает курс по slug.
func (r *Repository) GetCourseBySlug(slug string) (*Course, error) {
	c := &Course{}
	err := r.db.QueryRow(
		`SELECT id, slug, title, description, icon, order_index FROM courses WHERE slug = ?`,
		slug,
	).Scan(&c.ID, &c.Slug, &c.Title, &c.Description, &c.Icon, &c.OrderIndex)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get course by slug: %w", err)
	}
	return c, nil
}

// ListCourses возвращает все курсы.
func (r *Repository) ListCourses() ([]Course, error) {
	rows, err := r.db.Query(`SELECT id, slug, title, description, icon, order_index FROM courses ORDER BY order_index`)
	if err != nil {
		return nil, fmt.Errorf("list courses: %w", err)
	}
	defer rows.Close()

	var courses []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Slug, &c.Title, &c.Description, &c.Icon, &c.OrderIndex); err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		courses = append(courses, c)
	}

	return courses, rows.Err()
}

// --- Modules ---

// CreateModule создаёт новый модуль.
func (r *Repository) CreateModule(m *Module) error {
	_, err := r.db.Exec(
		`INSERT INTO modules (slug, title, order_index, course_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET title = excluded.title, order_index = excluded.order_index, course_id = excluded.course_id`,
		m.Slug, m.Title, m.OrderIndex, m.CourseID,
	)
	if err != nil {
		return fmt.Errorf("insert module: %w", err)
	}

	// Всегда получаем ID по slug (надёжнее чем LastInsertId при ON CONFLICT)
	err = r.db.QueryRow("SELECT id FROM modules WHERE slug = ?", m.Slug).Scan(&m.ID)
	if err != nil {
		return fmt.Errorf("get module id: %w", err)
	}

	return nil
}

// GetModuleBySlug возвращает модуль по slug.
func (r *Repository) GetModuleBySlug(slug string) (*Module, error) {
	m := &Module{}
	var courseID sql.NullInt64
	err := r.db.QueryRow(
		`SELECT id, slug, title, order_index, course_id FROM modules WHERE slug = ?`,
		slug,
	).Scan(&m.ID, &m.Slug, &m.Title, &m.OrderIndex, &courseID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get module by slug: %w", err)
	}
	if courseID.Valid {
		m.CourseID = courseID.Int64
	}
	return m, nil
}

// ListModules возвращает все модули.
func (r *Repository) ListModules() ([]Module, error) {
	rows, err := r.db.Query(`SELECT id, slug, title, order_index, COALESCE(course_id, 0) FROM modules ORDER BY order_index`)
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	defer rows.Close()

	var modules []Module
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.Slug, &m.Title, &m.OrderIndex, &m.CourseID); err != nil {
			return nil, fmt.Errorf("scan module: %w", err)
		}
		modules = append(modules, m)
	}

	return modules, rows.Err()
}

// ListModulesByCourseID возвращает модули для указанного курса.
func (r *Repository) ListModulesByCourseID(courseID int64) ([]Module, error) {
	rows, err := r.db.Query(
		`SELECT id, slug, title, order_index, COALESCE(course_id, 0) FROM modules WHERE course_id = ? ORDER BY order_index`,
		courseID,
	)
	if err != nil {
		return nil, fmt.Errorf("list modules by course: %w", err)
	}
	defer rows.Close()

	var modules []Module
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.Slug, &m.Title, &m.OrderIndex, &m.CourseID); err != nil {
			return nil, fmt.Errorf("scan module: %w", err)
		}
		modules = append(modules, m)
	}

	return modules, rows.Err()
}

// --- Lessons ---

// CreateLesson создаёт новый урок.
func (r *Repository) CreateLesson(l *Lesson) error {
	_, err := r.db.Exec(
		`INSERT INTO lessons (module_id, slug, title, order_index, source_url, body_md, reading_time_min)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET 
		   module_id = excluded.module_id,
		   title = excluded.title, 
		   order_index = excluded.order_index,
		   source_url = excluded.source_url,
		   body_md = excluded.body_md,
		   reading_time_min = excluded.reading_time_min,
		   updated_at = CURRENT_TIMESTAMP`,
		l.ModuleID, l.Slug, l.Title, l.OrderIndex, l.SourceURL, l.BodyMD, l.ReadingTimeMin,
	)
	if err != nil {
		return fmt.Errorf("insert lesson: %w", err)
	}

	// Всегда получаем ID по slug (надёжнее чем LastInsertId при ON CONFLICT)
	err = r.db.QueryRow("SELECT id FROM lessons WHERE slug = ?", l.Slug).Scan(&l.ID)
	if err != nil {
		return fmt.Errorf("get lesson id: %w", err)
	}

	return nil
}

// GetLessonBySlug возвращает урок по slug с секциями и заданиями.
func (r *Repository) GetLessonBySlug(slug string) (*Lesson, error) {
	l := &Lesson{Module: &Module{}}
	err := r.db.QueryRow(
		`SELECT l.id, l.module_id, l.slug, l.title, l.order_index, l.source_url, l.body_md, 
		        l.reading_time_min, l.created_at, l.updated_at,
		        m.id, m.slug, m.title, m.order_index
		 FROM lessons l
		 JOIN modules m ON m.id = l.module_id
		 WHERE l.slug = ?`,
		slug,
	).Scan(
		&l.ID, &l.ModuleID, &l.Slug, &l.Title, &l.OrderIndex, &l.SourceURL, &l.BodyMD,
		&l.ReadingTimeMin, &l.CreatedAt, &l.UpdatedAt,
		&l.Module.ID, &l.Module.Slug, &l.Module.Title, &l.Module.OrderIndex,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lesson by slug: %w", err)
	}

	// Загружаем секции
	l.Sections, err = r.GetSectionsByLessonID(l.ID)
	if err != nil {
		return nil, err
	}

	// Загружаем задания
	l.Tasks, err = r.GetTasksByLessonID(l.ID)
	if err != nil {
		return nil, err
	}

	return l, nil
}

// GetLessonByID возвращает урок по ID.
func (r *Repository) GetLessonByID(id int64) (*Lesson, error) {
	l := &Lesson{Module: &Module{}}
	err := r.db.QueryRow(
		`SELECT l.id, l.module_id, l.slug, l.title, l.order_index, l.source_url, l.body_md, 
		        l.reading_time_min, l.created_at, l.updated_at,
		        m.id, m.slug, m.title, m.order_index
		 FROM lessons l
		 JOIN modules m ON m.id = l.module_id
		 WHERE l.id = ?`,
		id,
	).Scan(
		&l.ID, &l.ModuleID, &l.Slug, &l.Title, &l.OrderIndex, &l.SourceURL, &l.BodyMD,
		&l.ReadingTimeMin, &l.CreatedAt, &l.UpdatedAt,
		&l.Module.ID, &l.Module.Slug, &l.Module.Title, &l.Module.OrderIndex,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lesson by id: %w", err)
	}

	l.Sections, _ = r.GetSectionsByLessonID(l.ID)
	l.Tasks, _ = r.GetTasksByLessonID(l.ID)

	return l, nil
}

// ListLessonsByModuleID возвращает уроки модуля.
func (r *Repository) ListLessonsByModuleID(moduleID int64) ([]Lesson, error) {
	rows, err := r.db.Query(
		`SELECT id, module_id, slug, title, order_index, source_url, body_md, reading_time_min, created_at, updated_at
		 FROM lessons WHERE module_id = ? ORDER BY order_index`,
		moduleID,
	)
	if err != nil {
		return nil, fmt.Errorf("list lessons: %w", err)
	}
	defer rows.Close()

	var lessons []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.ModuleID, &l.Slug, &l.Title, &l.OrderIndex,
			&l.SourceURL, &l.BodyMD, &l.ReadingTimeMin, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan lesson: %w", err)
		}
		lessons = append(lessons, l)
	}

	return lessons, rows.Err()
}

// ListAllLessons возвращает все уроки.
func (r *Repository) ListAllLessons() ([]Lesson, error) {
	rows, err := r.db.Query(
		`SELECT l.id, l.module_id, l.slug, l.title, l.order_index, l.source_url, l.body_md, 
		        l.reading_time_min, l.created_at, l.updated_at
		 FROM lessons l
		 JOIN modules m ON m.id = l.module_id
		 ORDER BY m.order_index, l.order_index`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all lessons: %w", err)
	}
	defer rows.Close()

	var lessons []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.ModuleID, &l.Slug, &l.Title, &l.OrderIndex,
			&l.SourceURL, &l.BodyMD, &l.ReadingTimeMin, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan lesson: %w", err)
		}
		lessons = append(lessons, l)
	}

	return lessons, rows.Err()
}

// --- Sections ---

// CreateSection создаёт секцию урока.
func (r *Repository) CreateSection(s *Section) error {
	result, err := r.db.Exec(
		`INSERT INTO lesson_sections (lesson_id, kind, title, body_md, order_index)
		 VALUES (?, ?, ?, ?, ?)`,
		s.LessonID, s.Kind, s.Title, s.BodyMD, s.OrderIndex,
	)
	if err != nil {
		return fmt.Errorf("insert section: %w", err)
	}

	s.ID, _ = result.LastInsertId()
	return nil
}

// DeleteSectionsByLessonID удаляет все секции урока.
func (r *Repository) DeleteSectionsByLessonID(lessonID int64) error {
	_, err := r.db.Exec(`DELETE FROM lesson_sections WHERE lesson_id = ?`, lessonID)
	return err
}

// GetSectionsByLessonID возвращает секции урока.
func (r *Repository) GetSectionsByLessonID(lessonID int64) ([]Section, error) {
	rows, err := r.db.Query(
		`SELECT id, lesson_id, kind, title, body_md, order_index 
		 FROM lesson_sections WHERE lesson_id = ? ORDER BY order_index`,
		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("get sections: %w", err)
	}
	defer rows.Close()

	var sections []Section
	for rows.Next() {
		var s Section
		if err := rows.Scan(&s.ID, &s.LessonID, &s.Kind, &s.Title, &s.BodyMD, &s.OrderIndex); err != nil {
			return nil, fmt.Errorf("scan section: %w", err)
		}
		sections = append(sections, s)
	}

	return sections, rows.Err()
}

// --- Tasks ---

// CreateTask создаёт задание.
func (r *Repository) CreateTask(t *Task) error {
	if strings.TrimSpace(t.Mode) == "" {
		t.Mode = "auto"
	}
	result, err := r.db.Exec(
		`INSERT INTO tasks (lesson_id, title, prompt_md, criteria, hints, starter_code, tests_go, expected_output, required_patterns, mode, points, order_index)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.LessonID, t.Title, t.PromptMD, t.Criteria, t.Hints, t.StarterCode, t.TestsGo, t.ExpectedOutput, t.RequiredPatterns, t.Mode, t.Points, t.OrderIndex,
	)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	t.ID, _ = result.LastInsertId()
	return nil
}

// DeleteTasksByLessonID удаляет все задания урока.
func (r *Repository) DeleteTasksByLessonID(lessonID int64) error {
	_, err := r.db.Exec(`DELETE FROM tasks WHERE lesson_id = ?`, lessonID)
	return err
}

// GetTasksByLessonID возвращает задания урока.
func (r *Repository) GetTasksByLessonID(lessonID int64) ([]Task, error) {
	rows, err := r.db.Query(
		`SELECT id, lesson_id, title, prompt_md, 
		        COALESCE(criteria, '') as criteria,
		        COALESCE(hints, '') as hints,
		        starter_code, tests_go, 
		        COALESCE(expected_output, '') as expected_output,
		        COALESCE(required_patterns, '') as required_patterns,
		        COALESCE(mode, 'auto') as mode,
		        points, order_index
		 FROM tasks WHERE lesson_id = ? ORDER BY order_index`,
		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.LessonID, &t.Title, &t.PromptMD, &t.Criteria, &t.Hints, &t.StarterCode, &t.TestsGo, &t.ExpectedOutput, &t.RequiredPatterns, &t.Mode, &t.Points, &t.OrderIndex); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

// GetTaskByID возвращает задание по ID.
func (r *Repository) GetTaskByID(id int64) (*Task, error) {
	t := &Task{}
	err := r.db.QueryRow(
		`SELECT id, lesson_id, title, prompt_md, 
		        COALESCE(criteria, '') as criteria,
		        COALESCE(hints, '') as hints,
		        starter_code, tests_go, 
		        COALESCE(expected_output, '') as expected_output, 
		        COALESCE(required_patterns, '') as required_patterns, 
		        COALESCE(mode, 'auto') as mode,
		        points, order_index
		 FROM tasks WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.LessonID, &t.Title, &t.PromptMD, &t.Criteria, &t.Hints, &t.StarterCode, &t.TestsGo, &t.ExpectedOutput, &t.RequiredPatterns, &t.Mode, &t.Points, &t.OrderIndex)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task by id: %w", err)
	}
	return t, nil
}

// --- Search ---

// Search выполняет полнотекстовый поиск по урокам.
func (r *Repository) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := r.db.Query(
		`SELECT l.id, l.slug, l.title, snippet(lessons_fts, 1, '<mark>', '</mark>', '...', 32) as snippet,
		        bm25(lessons_fts) as rank
		 FROM lessons_fts 
		 JOIN lessons l ON l.id = lessons_fts.rowid
		 WHERE lessons_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.LessonID, &r.Slug, &r.Title, &r.Snippet, &r.Rank); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}
