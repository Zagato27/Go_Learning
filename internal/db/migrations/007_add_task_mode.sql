-- Добавляем режим выполнения задания: auto (встроенная проверка) / manual (выполнение в IDE)
ALTER TABLE tasks ADD COLUMN mode TEXT NOT NULL DEFAULT 'auto';

