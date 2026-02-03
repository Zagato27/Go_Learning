// Go Learning — JavaScript

document.addEventListener('DOMContentLoaded', () => {
    initStatusButtons();
    initCodeEditors();
    initManualTasks();
    initNotesEditor();
});

// ========================================
// Status Buttons (прогресс урока)
// ========================================

function initStatusButtons() {
    document.querySelectorAll('.status-btn').forEach(btn => {
        btn.addEventListener('click', async () => {
            const lessonId = btn.dataset.lessonId;
            const status = btn.dataset.status;
            
            try {
                const response = await fetch(`/api/progress/lesson/${lessonId}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ status })
                });
                
                if (response.ok) {
                    // Обновляем UI
                    document.querySelectorAll('.status-btn').forEach(b => {
                        b.classList.remove('active');
                    });
                    btn.classList.add('active');
                }
            } catch (error) {
                console.error('Error updating status:', error);
            }
        });
    });
}

// ========================================
// Code Editors with CodeMirror
// ========================================

function initCodeEditors() {
    // Проверяем, доступен ли CodeMirror
    if (typeof CodeMirror === 'undefined') {
        console.warn('CodeMirror not loaded, falling back to textarea');
        initTaskActionsFallback();
        return;
    }

    document.querySelectorAll('.task-card').forEach(card => {
        const taskId = card.dataset.taskId;
        const textarea = card.querySelector('.code-input');
        const runBtn = card.querySelector('.run-btn');
        const checkBtn = card.querySelector('.check-btn');
        const outputDiv = card.querySelector('.task-output');
        const outputContent = card.querySelector('.output-content');
        
        if (!textarea) return;

        // Создаём CodeMirror редактор
        const editor = CodeMirror.fromTextArea(textarea, {
            mode: 'text/x-go',
            theme: 'monokai',
            lineNumbers: true,
            indentUnit: 4,
            tabSize: 4,
            indentWithTabs: true,
            matchBrackets: true,
            autoCloseBrackets: true,
            extraKeys: {
                'Tab': function(cm) {
                    cm.replaceSelection('\t');
                },
                'Ctrl-Enter': function() {
                    runBtn?.click();
                },
                'Ctrl-Shift-Enter': function() {
                    checkBtn?.click();
                }
            }
        });

        // Устанавливаем высоту
        editor.setSize(null, 250);

        // Функция получения кода
        const getCode = () => editor.getValue();

        // Запуск кода
        runBtn?.addEventListener('click', async () => {
            const code = getCode();
            
            runBtn.disabled = true;
            runBtn.textContent = '⏳ Запуск...';
            outputDiv.style.display = 'block';
            outputDiv.className = 'task-output';
            outputContent.textContent = 'Выполняется...';
            
            try {
                const response = await fetch('/api/run', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ code })
                });
                
                const result = await response.json();
                
                if (result.Success) {
                    outputDiv.className = 'task-output success';
                    outputContent.textContent = result.Stdout || 'Программа выполнена успешно (без вывода)';
                } else {
                    outputDiv.className = 'task-output error';
                    outputContent.textContent = result.Error || result.Stderr || 'Ошибка выполнения';
                }
            } catch (error) {
                outputDiv.className = 'task-output error';
                outputContent.textContent = 'Ошибка сети: ' + error.message;
            } finally {
                runBtn.disabled = false;
                runBtn.textContent = '▶ Запустить';
            }
        });
        
        // Проверка задания
        checkBtn?.addEventListener('click', async () => {
            const code = getCode();
            
            checkBtn.disabled = true;
            checkBtn.textContent = '⏳ Проверка...';
            outputDiv.style.display = 'block';
            outputDiv.className = 'task-output';
            outputContent.textContent = 'Проверяем...';
            
            try {
                const response = await fetch('/api/check', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ task_id: parseInt(taskId), code })
                });
                
                const result = await response.json();
                
                if (result.Success) {
                    outputDiv.className = 'task-output success';
                    let message = '✅ Задание выполнено правильно!';
                    if (result.PointsAwarded) {
                        message += `\n🏆 +${result.PointsAwarded} очков!`;
                    }
                    if (result.Output) {
                        message += '\n\n📤 Вывод программы:\n' + result.Output;
                    }
                    outputContent.textContent = message;
                    
                    // Обновляем бейдж очков на "Выполнено"
                    const pointsBadge = card.querySelector('.task-points');
                    if (pointsBadge && !pointsBadge.classList.contains('completed')) {
                        pointsBadge.textContent = '✅ Выполнено';
                        pointsBadge.classList.add('completed');
                    }
                    card.setAttribute('data-completed', 'true');
                    
                    // Обновляем статистику в шапке
                    updateHeaderStats();
                } else {
                    outputDiv.className = 'task-output error';
                    let message = '❌ ' + (result.Error || 'Решение неверное');
                    
                    // Показываем вывод программы если есть
                    if (result.Output) {
                        message += '\n\n📤 Ваш вывод:\n' + result.Output;
                    }
                    
                    // Показываем ожидаемый вывод если есть
                    if (result.Expected) {
                        message += '\n\n📋 Ожидаемый вывод:\n' + result.Expected;
                    }
                    
                    // Показываем подсказки если есть
                    if (result.Hints && result.Hints.length > 0) {
                        message += '\n\n💡 Подсказки:\n' + result.Hints.join('\n');
                    }
                    
                    outputContent.textContent = message;
                }
            } catch (error) {
                outputDiv.className = 'task-output error';
                outputContent.textContent = 'Ошибка сети: ' + error.message;
            } finally {
                checkBtn.disabled = false;
                checkBtn.textContent = '✓ Проверить';
            }
        });
    });
}

// ========================================
// Manual Tasks (выполнение в IDE)
// ========================================

function initManualTasks() {
    document.querySelectorAll('.task-card[data-task-mode="manual"]').forEach(card => {
        const taskId = card.dataset.taskId;
        const completeBtn = card.querySelector('.complete-btn');
        const outputDiv = card.querySelector('.task-output');
        const outputContent = card.querySelector('.output-content');

        if (!completeBtn || !taskId) return;

        completeBtn.addEventListener('click', async () => {
            completeBtn.disabled = true;
            const oldText = completeBtn.textContent;
            completeBtn.textContent = '⏳ Сохраняем...';

            if (outputDiv && outputContent) {
                outputDiv.style.display = 'block';
                outputDiv.className = 'task-output';
                outputContent.textContent = 'Отмечаем выполнение...';
            }

            try {
                const response = await fetch(`/api/tasks/${taskId}/complete`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' }
                });

                if (!response.ok) {
                    const text = await response.text();
                    throw new Error(text || `HTTP ${response.status}`);
                }

                const result = await response.json();

                const points = result.points_awarded || 0;

                if (outputDiv && outputContent) {
                    outputDiv.className = 'task-output success';
                    let message = '✅ Задание отмечено выполненным.';
                    if (points) {
                        message += `\n🏆 +${points} очков!`;
                    }
                    outputContent.textContent = message;
                }

                // Обновляем бейдж очков на "Выполнено"
                const pointsBadge = card.querySelector('.task-points');
                if (pointsBadge && !pointsBadge.classList.contains('completed')) {
                    pointsBadge.textContent = '✅ Выполнено';
                    pointsBadge.classList.add('completed');
                }
                card.setAttribute('data-completed', 'true');

                // Закрываем кнопку (или оставляем disabled)
                completeBtn.textContent = '✅ Выполнено';

                // Обновляем статистику в шапке
                updateHeaderStats();
            } catch (error) {
                completeBtn.disabled = false;
                completeBtn.textContent = oldText;

                if (outputDiv && outputContent) {
                    outputDiv.className = 'task-output error';
                    outputContent.textContent = 'Ошибка: ' + error.message;
                }
            }
        });
    });
}

// Обновление статистики в шапке после получения очков
async function updateHeaderStats() {
    try {
        // Перезагружаем страницу чтобы обновить статистику
        // В будущем можно сделать AJAX-запрос для получения только статистики
        setTimeout(() => {
            window.location.reload();
        }, 1500);
    } catch (error) {
        console.error('Error updating stats:', error);
    }
}

// Fallback если CodeMirror не загрузился
function initTaskActionsFallback() {
    document.querySelectorAll('.task-card').forEach(card => {
        const taskId = card.dataset.taskId;
        const codeInput = card.querySelector('.code-input');
        const runBtn = card.querySelector('.run-btn');
        const checkBtn = card.querySelector('.check-btn');
        const outputDiv = card.querySelector('.task-output');
        const outputContent = card.querySelector('.output-content');
        
        runBtn?.addEventListener('click', async () => {
            const code = codeInput.value;
            
            runBtn.disabled = true;
            runBtn.textContent = '⏳ Запуск...';
            outputDiv.style.display = 'block';
            outputDiv.className = 'task-output';
            outputContent.textContent = 'Выполняется...';
            
            try {
                const response = await fetch('/api/run', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ code })
                });
                
                const result = await response.json();
                
                if (result.Success) {
                    outputDiv.className = 'task-output success';
                    outputContent.textContent = result.Stdout || 'Программа выполнена успешно (без вывода)';
                } else {
                    outputDiv.className = 'task-output error';
                    outputContent.textContent = result.Error || result.Stderr || 'Ошибка выполнения';
                }
            } catch (error) {
                outputDiv.className = 'task-output error';
                outputContent.textContent = 'Ошибка сети: ' + error.message;
            } finally {
                runBtn.disabled = false;
                runBtn.textContent = '▶ Запустить';
            }
        });
        
        checkBtn?.addEventListener('click', async () => {
            const code = codeInput.value;
            
            checkBtn.disabled = true;
            checkBtn.textContent = '⏳ Проверка...';
            outputDiv.style.display = 'block';
            outputDiv.className = 'task-output';
            outputContent.textContent = 'Проверяем...';
            
            try {
                const response = await fetch('/api/check', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ task_id: parseInt(taskId), code })
                });
                
                const result = await response.json();
                
                if (result.Success) {
                    outputDiv.className = 'task-output success';
                    let message = '✅ Задание выполнено правильно!';
                    if (result.PointsAwarded) {
                        message += `\n🏆 +${result.PointsAwarded} очков!`;
                    }
                    if (result.Output) {
                        message += '\n\n📤 Вывод программы:\n' + result.Output;
                    }
                    outputContent.textContent = message;
                    
                    // Обновляем бейдж очков на "Выполнено"
                    const pointsBadge = card.querySelector('.task-points');
                    if (pointsBadge && !pointsBadge.classList.contains('completed')) {
                        pointsBadge.textContent = '✅ Выполнено';
                        pointsBadge.classList.add('completed');
                    }
                    card.setAttribute('data-completed', 'true');
                    
                    updateHeaderStats();
                } else {
                    outputDiv.className = 'task-output error';
                    let message = '❌ ' + (result.Error || 'Решение неверное');
                    
                    if (result.Output) {
                        message += '\n\n📤 Ваш вывод:\n' + result.Output;
                    }
                    if (result.Expected) {
                        message += '\n\n📋 Ожидаемый вывод:\n' + result.Expected;
                    }
                    if (result.Hints && result.Hints.length > 0) {
                        message += '\n\n💡 Подсказки:\n' + result.Hints.join('\n');
                    }
                    
                    outputContent.textContent = message;
                }
            } catch (error) {
                outputDiv.className = 'task-output error';
                outputContent.textContent = 'Ошибка сети: ' + error.message;
            } finally {
                checkBtn.disabled = false;
                checkBtn.textContent = '✓ Проверить';
            }
        });
    });
}

// ========================================
// Notes Editor
// ========================================

function initNotesEditor() {
    const notesInput = document.querySelector('.notes-input');
    const saveBtn = document.querySelector('.save-notes-btn');
    const statusSpan = document.querySelector('.notes-status');
    
    if (!notesInput || !saveBtn) return;
    
    const lessonId = notesInput.dataset.lessonId;
    let saveTimeout = null;
    
    // Автосохранение при изменении
    notesInput.addEventListener('input', () => {
        statusSpan.textContent = 'Несохранённые изменения...';
        
        // Отложенное автосохранение
        clearTimeout(saveTimeout);
        saveTimeout = setTimeout(() => saveNotes(), 2000);
    });
    
    // Кнопка сохранения
    saveBtn.addEventListener('click', saveNotes);
    
    async function saveNotes() {
        clearTimeout(saveTimeout);
        
        const note = notesInput.value;
        statusSpan.textContent = 'Сохранение...';
        
        try {
            const response = await fetch(`/api/notes/lesson/${lessonId}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ note })
            });
            
            if (response.ok) {
                statusSpan.textContent = '✓ Сохранено';
                setTimeout(() => {
                    statusSpan.textContent = '';
                }, 2000);
            } else {
                statusSpan.textContent = '❌ Ошибка сохранения';
            }
        } catch (error) {
            statusSpan.textContent = '❌ Ошибка сети';
        }
    }
}

// ========================================
// Reset Progress
// ========================================

async function resetProgress() {
    if (!confirm('Вы уверены, что хотите сбросить весь прогресс обучения?\n\nБудут удалены:\n• Все отметки о прохождении уроков\n• Все заработанные очки\n• Все отправленные решения\n\nЗаметки к урокам сохранятся.')) {
        return;
    }
    
    try {
        const response = await fetch('/api/progress/reset', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        
        if (response.ok) {
            alert('✅ Прогресс успешно сброшен!');
            window.location.reload();
        } else {
            alert('❌ Ошибка при сбросе прогресса');
        }
    } catch (error) {
        alert('❌ Ошибка сети: ' + error.message);
    }
}
