// 获取存储的 token
function getToken() {
    return localStorage.getItem('token');
}

// 设置 token
function setToken(token) {
    localStorage.setItem('token', token);
    // 同时设置 cookie 用于页面访问
    document.cookie = `token=${token}; path=/; max-age=86400`;
}

// 清除 token
function clearToken() {
    localStorage.removeItem('token');
    // 清除 cookie
    document.cookie = 'token=; path=/; max-age=0';
}

// 获取 Authorization header
function getAuthHeader() {
    const token = getToken();
    return token ? { 'Authorization': `Bearer ${token}` } : {};
}

// 显示错误消息
function showError(message) {
    const errorEl = document.getElementById('error-message');
    if (errorEl) {
        errorEl.textContent = message;
        errorEl.classList.remove('hidden');
        setTimeout(() => errorEl.classList.add('hidden'), 5000);
    }
}

// 显示成功消息
function showSuccess(message) {
    const successEl = document.getElementById('success-message');
    if (successEl) {
        successEl.textContent = message;
        successEl.classList.remove('hidden');
        setTimeout(() => successEl.classList.add('hidden'), 5000);
    }
}

// 显示消息（文件页面）
function showMessage(message, isError = false) {
    const messageEl = document.getElementById('message');
    if (messageEl) {
        messageEl.textContent = message;
        messageEl.classList.remove('hidden');
        messageEl.className = `mb-4 p-3 rounded-md ${isError ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'}`;
        setTimeout(() => messageEl.classList.add('hidden'), 5000);
    }
}

// ========== 上传进度水波球 ==========
function getProgressBall() {
    return document.getElementById('upload-progress-ball');
}

function getProgressWater() {
    return document.querySelector('#upload-progress-ball .water');
}

function getProgressText() {
    return document.querySelector('#upload-progress-ball .percent-text');
}

function showProgressBall() {
    var ball = getProgressBall();
    if (ball) {
        ball.classList.add('visible');
    }
    updateProgressBall(0);
}

function hideProgressBall(delay) {
    delay = delay || 0;
    setTimeout(function() {
        var ball = getProgressBall();
        if (ball) {
            ball.classList.remove('visible');
        }
        // 重置状态
        setTimeout(function() {
            updateProgressBall(0);
            var water = getProgressWater();
            if (water) {
                water.style.background = 'linear-gradient(180deg, #5ca0e8 0%, #2670c4 100%)';
            }
            var text = getProgressText();
            if (text) {
                text.style.color = '#3b82f6';
            }
        }, 500);
    }, delay);
}

function updateProgressBall(percent) {
    var water = getProgressWater();
    var text = getProgressText();
    if (water) {
        water.style.height = percent + '%';
    }
    if (text) {
        text.textContent = Math.round(percent) + '%';
        // 进度高于 50% 时文字变白色以保持可读性
        if (percent > 50) {
            text.style.color = '#ffffff';
        } else {
            text.style.color = '#3b82f6';
        }
    }
}

function setProgressBallError() {
    var water = getProgressWater();
    var text = getProgressText();
    if (water) {
        water.style.background = 'linear-gradient(180deg, #f87171 0%, #dc2626 100%)';
    }
    if (text) {
        text.style.color = '#ffffff';
        text.textContent = '失败';
    }
}
// ========== 上传进度水波球 END ==========

// 登录处理
document.addEventListener('DOMContentLoaded', function() {
    const loginForm = document.getElementById('login-form');
    if (loginForm) {
        loginForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            
            try {
                const response = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ username, password }),
                });
                
                const data = await response.json();
                
                if (response.ok) {
                    setToken(data.token);
                    window.location.href = '/files';
                } else {
                    showError(data.error || '登录失败');
                }
            } catch (err) {
                showError('网络错误，请重试');
            }
        });
    }
    
    // 注册处理
    const registerForm = document.getElementById('register-form');
    if (registerForm) {
        registerForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            const confirmPassword = document.getElementById('confirm-password').value;
            
            if (password !== confirmPassword) {
                showError('两次输入的密码不一致');
                return;
            }
            
            try {
                const response = await fetch('/api/auth/register', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ username, password }),
                });
                
                const data = await response.json();
                
                if (response.ok) {
                    showSuccess('注册成功，即将跳转到登录页面');
                    setTimeout(() => {
                        window.location.href = '/login';
                    }, 2000);
                } else {
                    showError(data.error || '注册失败');
                }
            } catch (err) {
                showError('网络错误，请重试');
            }
        });
    }
    
    // 使用 Resumable.js 实现断续上传
    const fileInput = document.getElementById('file-input');
    if (fileInput) {
        var r = new Resumable({
            target: '/api/files/upload',
            testTarget: '/api/files/upload',
            testChunks: true,
            chunkSize: 1 * 1024 * 1024, // 1MB
            simultaneousUploads: 3,
            headers: getAuthHeader(),
            query: function(file, chunk) {
                // 附加 parent_id 到每个请求
                var parentInput = document.querySelector('input[name="parent_id"]');
                var params = {};
                if (parentInput && parentInput.value) {
                    params.parent_id = parentInput.value;
                }
                return params;
            }
        });

        r.assignBrowse(fileInput);

        r.on('fileAdded', function(file) {
            showMessage('正在上传: ' + file.fileName);
            showProgressBall();
            r.upload();
        });

        r.on('fileProgress', function(file) {
            var percent = file.progress(false) * 100;
            updateProgressBall(percent);
        });

        r.on('fileSuccess', function(file) {
            showMessage(file.fileName + ' 上传成功');
            updateProgressBall(100);
            hideProgressBall(1500);
            setTimeout(function() { window.location.reload(); }, 2000);
        });

        r.on('fileError', function(file, message) {
            showMessage(file.fileName + ' 上传失败: ' + message, true);
            setProgressBallError();
            hideProgressBall(3000);
        });

        r.on('error', function(message, file) {
            showMessage('上传出错: ' + message, true);
            setProgressBallError();
            hideProgressBall(3000);
        });

        r.on('complete', function() {
            updateProgressBall(100);
            hideProgressBall(1500);
            setTimeout(function() { window.location.reload(); }, 2000);
        });
    }
    
    // 创建文件夹处理
    const createFolderForm = document.getElementById('create-folder-form');
    if (createFolderForm) {
        createFolderForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const name = document.getElementById('folder-name').value;
            const parentInput = document.querySelector('#create-folder-form input[name="parent_id"]');
            
            const body = { name: name };
            if (parentInput && parentInput.value) {
                body.parent_id = parseInt(parentInput.value);
            }
            
            try {
                const response = await fetch('/api/folders', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        ...getAuthHeader(),
                    },
                    body: JSON.stringify(body),
                });
                
                const data = await response.json();
                
                if (response.ok) {
                    showMessage('文件夹创建成功');
                    hideCreateFolderModal();
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showMessage(data.error || '创建失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }
});

// 显示创建文件夹模态框
function showCreateFolderModal() {
    const modal = document.getElementById('create-folder-modal');
    if (modal) {
        modal.classList.remove('hidden');
    }
}

// 隐藏创建文件夹模态框
function hideCreateFolderModal() {
    const modal = document.getElementById('create-folder-modal');
    const input = document.getElementById('folder-name');
    if (modal) {
        modal.classList.add('hidden');
    }
    if (input) {
        input.value = '';
    }
}

// 删除文件/文件夹
async function deleteItem(id, isFolder) {
    if (!confirm('确定要删除吗？')) {
        return;
    }
    
    try {
        const response = await fetch(`/api/files/${id}`, {
            method: 'DELETE',
            headers: getAuthHeader(),
        });
        
        const data = await response.json();
        
        if (response.ok) {
            showMessage('删除成功');
            setTimeout(() => window.location.reload(), 1000);
        } else {
            showMessage(data.error || '删除失败', true);
        }
    } catch (err) {
        showMessage('网络错误，请重试', true);
    }
}

// 下载文件
function downloadFile(id) {
    window.location.href = `/api/files/${id}/download`;
}

// 退出登录
function logout() {
    clearToken();
    window.location.href = '/login';
}

// 切换下拉菜单
function toggleDropdown(id) {
    // 关闭其他下拉菜单
    document.querySelectorAll('[id^="dropdown-"]').forEach(el => {
        if (el.id !== id) {
            el.classList.add('hidden');
        }
    });
    
    const dropdown = document.getElementById(id);
    if (dropdown) {
        dropdown.classList.toggle('hidden');
    }
}

// 点击页面其他地方关闭下拉菜单
document.addEventListener('click', function(e) {
    if (!e.target.closest('[onclick^="toggleDropdown"]') && !e.target.closest('[id^="dropdown-"]')) {
        document.querySelectorAll('[id^="dropdown-"]').forEach(el => {
            el.classList.add('hidden');
        });
    }
});

// 显示重命名模态框
function showRenameModal(id, currentName) {
    const modal = document.getElementById('rename-modal');
    const idInput = document.getElementById('rename-item-id');
    const nameInput = document.getElementById('rename-item-name');
    
    if (modal && idInput && nameInput) {
        idInput.value = id;
        nameInput.value = currentName;
        modal.classList.remove('hidden');
        nameInput.focus();
        nameInput.select();
    }
    
    // 关闭下拉菜单
    document.querySelectorAll('[id^="dropdown-"]').forEach(el => {
        el.classList.add('hidden');
    });
}

// 从链接元素显示重命名模态框（处理特殊字符）
function showRenameModalFromLink(element) {
    const id = element.getAttribute('data-id');
    const name = element.getAttribute('data-name');
    showRenameModal(id, name);
}

// 隐藏重命名模态框
function hideRenameModal() {
    const modal = document.getElementById('rename-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
}

// 重命名表单提交处理
document.addEventListener('DOMContentLoaded', function() {
    const renameForm = document.getElementById('rename-form');
    if (renameForm) {
        renameForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const id = document.getElementById('rename-item-id').value;
            const newName = document.getElementById('rename-item-name').value;
            
            try {
                const response = await fetch(`/api/files/${id}/rename`, {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json',
                        ...getAuthHeader(),
                    },
                    body: JSON.stringify({ name: newName }),
                });
                
                const data = await response.json();
                
                if (response.ok) {
                    showMessage('重命名成功');
                    hideRenameModal();
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showMessage(data.error || '重命名失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }
});

// 显示移动模态框
async function showMoveModal(element) {
    const id = element.getAttribute('data-id');
    const modal = document.getElementById('move-modal');
    const idInput = document.getElementById('move-item-id');
    const select = document.getElementById('move-target-id');

    if (modal && idInput && select) {
        idInput.value = id;

        // 关闭下拉菜单
        document.querySelectorAll('[id^="dropdown-"]').forEach(el => {
            el.classList.add('hidden');
        });

        // 清空并加载文件夹列表
        select.innerHTML = '<option value="">根目录</option>';

        try {
            const response = await fetch('/api/folders/all', {
                headers: getAuthHeader(),
            });
            const data = await response.json();

            if (response.ok && data.folders) {
                // 构建 id -> folder 映射
                const folderMap = {};
                data.folders.forEach(function(f) {
                    folderMap[f.ID] = f;
                });

                // 为每个文件夹构建完整路径
                data.folders.forEach(function(f) {
                    // 排除自身（不能移动到自己里面）
                    if (String(f.ID) === String(id)) return;

                    var path = buildFolderPath(f, folderMap);
                    var option = document.createElement('option');
                    option.value = f.ID;
                    option.textContent = path;
                    select.appendChild(option);
                });
            }
        } catch (err) {
            // 加载失败不影响使用，用户至少可以选根目录
        }

        modal.classList.remove('hidden');
    }
}

// 构建文件夹的完整路径
function buildFolderPath(folder, folderMap) {
    var parts = [folder.Name];
    var current = folder;
    var visited = {};

    while (current.ParentID && folderMap[current.ParentID]) {
        if (visited[current.ID]) break;
        visited[current.ID] = true;

        current = folderMap[current.ParentID];
        parts.unshift(current.Name);
    }

    return '根目录 / ' + parts.join(' / ');
}

// 隐藏移动模态框
function hideMoveModal() {
    const modal = document.getElementById('move-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
}

// 移动表单提交处理
document.addEventListener('DOMContentLoaded', function() {
    const moveForm = document.getElementById('move-form');
    if (moveForm) {
        moveForm.addEventListener('submit', async function(e) {
            e.preventDefault();

            const id = document.getElementById('move-item-id').value;
            const targetValue = document.getElementById('move-target-id').value;
            const body = {};
            if (targetValue) {
                body.target_id = parseInt(targetValue);
            }

            try {
                const response = await fetch(`/api/files/${id}/move`, {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json',
                        ...getAuthHeader(),
                    },
                    body: JSON.stringify(body),
                });

                const data = await response.json();

                if (response.ok) {
                    showMessage('移动成功');
                    hideMoveModal();
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showMessage(data.error || '移动失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }
});
