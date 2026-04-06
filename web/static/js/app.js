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
    
    // 上传文件处理
    const fileInput = document.getElementById('file-input');
    if (fileInput) {
        fileInput.addEventListener('change', async function(e) {
            const file = e.target.files[0];
            if (!file) return;
            
            const formData = new FormData();
            formData.append('file', file);
            
            // 检查是否有 parent_id
            const parentInput = document.querySelector('input[name="parent_id"]');
            if (parentInput) {
                formData.append('parent_id', parentInput.value);
            }
            
            try {
                const response = await fetch('/api/files/upload', {
                    method: 'POST',
                    headers: getAuthHeader(),
                    body: formData,
                });
                
                const data = await response.json();
                
                if (response.ok) {
                    showMessage('文件上传成功');
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showMessage(data.error || '上传失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
            
            // 清空文件选择
            fileInput.value = '';
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
async function downloadFile(id) {
    try {
        const response = await fetch(`/api/files/${id}/download`, {
            method: 'GET',
            headers: getAuthHeader(),
        });
        
        if (!response.ok) {
            const data = await response.json();
            showMessage(data.error || '下载失败', true);
            return;
        }
        
        // 获取文件名
        const contentDisposition = response.headers.get('Content-Disposition');
        let filename = 'download';
        if (contentDisposition) {
            const match = contentDisposition.match(/filename=(.+)/);
            if (match) {
                filename = match[1];
            }
        }
        
        // 下载文件
        const blob = await response.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        document.body.removeChild(a);
    } catch (err) {
        showMessage('网络错误，请重试', true);
    }
}

// 退出登录
function logout() {
    clearToken();
    window.location.href = '/login';
}
