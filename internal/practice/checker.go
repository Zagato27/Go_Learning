package practice

import (
	"context"
	"fmt"
	"strings"

	"golearning/internal/content"
	"golearning/internal/progress"
)

// Checker — сервис проверки решений.
type Checker struct {
	runner       Runner
	contentRepo  *content.Repository
	progressRepo *progress.Repository
}

// NewChecker создаёт новый checker.
func NewChecker(runner Runner, contentRepo *content.Repository, progressRepo *progress.Repository) *Checker {
	return &Checker{
		runner:       runner,
		contentRepo:  contentRepo,
		progressRepo: progressRepo,
	}
}

// CheckResult — результат проверки задания.
type CheckResult struct {
	Success       bool
	Output        string
	Expected      string
	Error         string
	Hints         []string
	PointsAwarded int
}

// Check проверяет решение задания.
func (c *Checker) Check(ctx context.Context, taskID int64, code string) (*CheckResult, error) {
	// Получаем задание
	task, err := c.contentRepo.GetTaskByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return &CheckResult{
			Success: false,
			Error:   "Задание не найдено",
		}, nil
	}

	// Manual-задачи выполняются вне встроенного редактора.
	if strings.TrimSpace(task.Mode) == "manual" {
		return &CheckResult{
			Success: false,
			Error:   "Это ручное задание. Выполните его в IDE и нажмите «Отметить выполненным».",
		}, nil
	}

	// Создаём запись о submissions
	submission := &progress.Submission{
		TaskID: taskID,
		Code:   code,
		Status: "pending",
	}
	if err := c.progressRepo.CreateSubmission(submission); err != nil {
		return nil, fmt.Errorf("create submission: %w", err)
	}

	checkResult := &CheckResult{
		Hints: []string{},
	}

	// Шаг 1: Проверяем обязательные паттерны в коде
	if task.RequiredPatterns != "" {
		patterns := strings.Split(task.RequiredPatterns, "|")
		missingPatterns := []string{}
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" && !strings.Contains(code, pattern) {
				missingPatterns = append(missingPatterns, pattern)
			}
		}
		if len(missingPatterns) > 0 {
			submission.Status = "error"
			checkResult.Success = false
			checkResult.Error = "В коде отсутствуют необходимые конструкции"
			checkResult.Hints = append(checkResult.Hints, fmt.Sprintf("Используйте: %s", strings.Join(missingPatterns, ", ")))
			c.progressRepo.UpdateSubmission(submission)
			return checkResult, nil
		}
	}

	// Шаг 2: Запускаем код
	runResult, err := c.runner.Run(ctx, code)
	if err != nil {
		submission.Status = "error"
		submission.Stderr = err.Error()
		c.progressRepo.UpdateSubmission(submission)
		return nil, fmt.Errorf("run code: %w", err)
	}

	// Если код не компилируется
	if !runResult.Success {
		submission.Status = "error"
		submission.Stderr = runResult.Error
		checkResult.Success = false
		checkResult.Output = runResult.Stdout
		checkResult.Error = runResult.Error
		c.progressRepo.UpdateSubmission(submission)
		return checkResult, nil
	}

	submission.Stdout = runResult.Stdout
	checkResult.Output = runResult.Stdout

	// Шаг 3: Проверяем ожидаемый вывод
	if task.ExpectedOutput != "" {
		actualOutput := strings.TrimSpace(runResult.Stdout)
		expectedOutput := strings.TrimSpace(task.ExpectedOutput)
		checkResult.Expected = expectedOutput

		if !c.compareOutput(actualOutput, expectedOutput) {
			submission.Status = "error"
			checkResult.Success = false
			checkResult.Error = "Вывод программы не соответствует ожидаемому"
			checkResult.Hints = append(checkResult.Hints, fmt.Sprintf("Ожидалось:\n%s", expectedOutput))
			c.progressRepo.UpdateSubmission(submission)
			return checkResult, nil
		}
	}

	// Шаг 4: Если есть тесты — запускаем их
	if task.TestsGo != "" {
		testResult, err := c.runner.Check(ctx, code, task.TestsGo)
		if err != nil {
			submission.Status = "error"
			submission.Stderr = err.Error()
			c.progressRepo.UpdateSubmission(submission)
			return nil, fmt.Errorf("run tests: %w", err)
		}

		if !testResult.Success {
			submission.Status = "error"
			submission.Stderr = testResult.Error
			checkResult.Success = false
			checkResult.Error = "Тесты не пройдены"
			if testResult.Error != "" {
				checkResult.Hints = append(checkResult.Hints, testResult.Error)
			}
			c.progressRepo.UpdateSubmission(submission)
			return checkResult, nil
		}
	}

	// Все проверки пройдены!
	checkResult.Success = true
	submission.Status = "success"

	// Проверяем, было ли задание уже решено ранее
	alreadySolved, _ := c.progressRepo.IsTaskSolvedSuccessfully(taskID)

	if !alreadySolved {
		// Начисляем очки только при первом успешном решении
		checkResult.PointsAwarded = task.Points
		if err := c.progressRepo.SetPracticeDone(task.LessonID, task.Points); err != nil {
			// Не критично, продолжаем
		}
	}

	c.progressRepo.UpdateSubmission(submission)
	return checkResult, nil
}

// compareOutput сравнивает фактический и ожидаемый вывод.
// Поддерживает гибкое сравнение (игнорирует лишние пробелы, пустые строки).
func (c *Checker) compareOutput(actual, expected string) bool {
	// Нормализуем строки
	actual = c.normalizeOutput(actual)
	expected = c.normalizeOutput(expected)

	// Точное совпадение
	if actual == expected {
		return true
	}

	// Сравнение построчно (игнорируя пустые строки)
	actualLines := c.nonEmptyLines(actual)
	expectedLines := c.nonEmptyLines(expected)

	if len(actualLines) != len(expectedLines) {
		return false
	}

	for i := range actualLines {
		if strings.TrimSpace(actualLines[i]) != strings.TrimSpace(expectedLines[i]) {
			return false
		}
	}

	return true
}

// normalizeOutput нормализует вывод для сравнения.
func (c *Checker) normalizeOutput(s string) string {
	// Заменяем Windows-переносы на Unix
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Убираем trailing whitespace
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// nonEmptyLines возвращает непустые строки.
func (c *Checker) nonEmptyLines(s string) []string {
	lines := strings.Split(s, "\n")
	result := []string{}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

// Run просто выполняет код без проверки.
func (c *Checker) Run(ctx context.Context, code string) (*RunResult, error) {
	return c.runner.Run(ctx, code)
}
