// ========== Token 管理（仅内存） ==========

var _accessToken = window.__ACCESS_TOKEN__ || '';

function getAccessToken() {
    return _accessToken;
}

function setAccessToken(token) {
    _accessToken = token;
}

function getAuthHeader() {
    var token = getAccessToken();
    return token ? { 'Authorization': 'Bearer ' + token } : {};
}

// 带自动刷新的 fetch 封装
function authFetch(url, options) {
    options = options || {};
    var headers = Object.assign({}, options.headers || {}, getAuthHeader());
    options.headers = headers;

    return fetch(url, options).then(function(response) {
        if (response.status === 401 && _accessToken) {
            // access token 过期，尝试刷新
            return fetch('/api/auth/refresh', { method: 'POST' }).then(function(refreshRes) {
                if (refreshRes.ok) {
                    return refreshRes.json().then(function(data) {
                        setAccessToken(data.access_token);
                        // 重试原请求
                        headers = Object.assign({}, options.headers || {}, getAuthHeader());
                        options.headers = headers;
                        return fetch(url, options);
                    });
                }
                // refresh 也失败，跳转登录
                window.location.href = '/login';
                return Promise.reject(new Error('session expired'));
            });
        }
        return response;
    });
}

// ========== 消息提示 ==========

function showError(message) {
    var errorEl = document.getElementById('error-message');
    if (errorEl) {
        errorEl.textContent = message;
        errorEl.classList.remove('hidden');
        setTimeout(function() { errorEl.classList.add('hidden'); }, 5000);
    }
}

function showSuccess(message) {
    var successEl = document.getElementById('success-message');
    if (successEl) {
        successEl.textContent = message;
        successEl.classList.remove('hidden');
        setTimeout(function() { successEl.classList.add('hidden'); }, 5000);
    }
}

function showMessage(message, isError) {
    var messageEl = document.getElementById('message');
    if (messageEl) {
        messageEl.textContent = message;
        messageEl.classList.remove('hidden');
        messageEl.className = 'mb-4 p-3 rounded-md ' + (isError ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700');
        setTimeout(function() { messageEl.classList.add('hidden'); }, 5000);
    }
}

// ========== 上传进度水波球 ==========

function getProgressBall() { return document.getElementById('upload-progress-ball'); }
function getProgressWater() { return document.querySelector('#upload-progress-ball .water'); }
function getProgressText() { return document.querySelector('#upload-progress-ball .percent-text'); }

function showProgressBall() {
    var ball = getProgressBall();
    if (ball) ball.classList.add('visible');
    updateProgressBall(0);
}

function hideProgressBall(delay) {
    delay = delay || 0;
    setTimeout(function() {
        var ball = getProgressBall();
        if (ball) ball.classList.remove('visible');
        setTimeout(function() {
            updateProgressBall(0);
            var water = getProgressWater();
            if (water) water.style.background = 'linear-gradient(180deg, #5ca0e8 0%, #2670c4 100%)';
            var text = getProgressText();
            if (text) text.style.color = '#3b82f6';
        }, 500);
    }, delay);
}

function updateProgressBall(percent) {
    var water = getProgressWater();
    var text = getProgressText();
    if (water) water.style.height = percent + '%';
    if (text) {
        text.textContent = Math.round(percent) + '%';
        text.style.color = percent > 50 ? '#ffffff' : '#3b82f6';
    }
}

function setProgressBallError() {
    var water = getProgressWater();
    var text = getProgressText();
    if (water) water.style.background = 'linear-gradient(180deg, #f87171 0%, #dc2626 100%)';
    if (text) { text.style.color = '#ffffff'; text.textContent = '失败'; }
}

// ========== 登录/注册 ==========

document.addEventListener('DOMContentLoaded', function() {
    var loginForm = document.getElementById('login-form');
    if (loginForm) {
        loginForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            var username = document.getElementById('username').value;
            var password = document.getElementById('password').value;

            try {
                var response = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username: username, password: password }),
                });
                var data = await response.json();
                if (response.ok) {
                    setAccessToken(data.access_token);
                    window.location.href = '/files';
                } else {
                    showError(data.error || '登录失败');
                }
            } catch (err) {
                showError('网络错误，请重试');
            }
        });
    }

    var registerForm = document.getElementById('register-form');
    if (registerForm) {
        registerForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            var username = document.getElementById('username').value;
            var password = document.getElementById('password').value;
            var confirmPassword = document.getElementById('confirm-password').value;

            if (password !== confirmPassword) {
                showError('两次输入的密码不一致');
                return;
            }

            try {
                var response = await fetch('/api/auth/register', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username: username, password: password }),
                });
                var data = await response.json();
                if (response.ok) {
                    showSuccess('注册成功，即将跳转到登录页面');
                    setTimeout(function() { window.location.href = '/login'; }, 2000);
                } else {
                    showError(data.error || '注册失败');
                }
            } catch (err) {
                showError('网络错误，请重试');
            }
        });
    }

    // ========== 文件上传（Resumable.js） ==========
    var fileInput = document.getElementById('file-input');
    if (fileInput) {
        var _uploadRefreshTimer = null;
        var _retrying401 = {};

        function startUploadRefresh() {
            if (_uploadRefreshTimer) return;
            _uploadRefreshTimer = setInterval(function() {
                fetch('/api/auth/refresh', { method: 'POST' }).then(function(res) {
                    if (res.ok) {
                        return res.json().then(function(data) { setAccessToken(data.access_token); });
                    }
                });
            }, 4 * 60 * 1000);
        }

        function stopUploadRefresh() {
            if (_uploadRefreshTimer) {
                clearInterval(_uploadRefreshTimer);
                _uploadRefreshTimer = null;
            }
        }

        var r = new Resumable({
            target: '/api/files/upload',
            testTarget: '/api/files/upload',
            testChunks: true,
            chunkSize: 1 * 1024 * 1024,
            simultaneousUploads: 3,
            headers: function() { return getAuthHeader(); },
            query: function(file, chunk) {
                var parentInput = document.querySelector('input[name="parent_id"]');
                var params = {};
                if (parentInput && parentInput.value) params.parent_id = parentInput.value;
                return params;
            }
        });

        r.assignBrowse(fileInput);

        r.on('fileAdded', function(file) {
            showMessage('正在上传: ' + file.fileName);
            showProgressBall();
            startUploadRefresh();
            r.upload();
        });

        r.on('fileProgress', function(file) {
            updateProgressBall(file.progress(false) * 100);
        });

        r.on('fileSuccess', function(file) {
            showMessage(file.fileName + ' 上传成功');
            updateProgressBall(100);
            hideProgressBall(1500);
            setTimeout(function() { window.location.reload(); }, 2000);
        });

        r.on('fileError', function(file, message) {
            var isAuthError = false;
            try {
                var errData = JSON.parse(message);
                if (errData.error && (errData.error.indexOf('token') !== -1 || errData.error.indexOf('authorization') !== -1)) {
                    isAuthError = true;
                }
            } catch(e) {}
            if (!_retrying401[file.uniqueIdentifier] && isAuthError) {
                _retrying401[file.uniqueIdentifier] = true;
                fetch('/api/auth/refresh', { method: 'POST' }).then(function(res) {
                    if (res.ok) {
                        return res.json().then(function(data) {
                            setAccessToken(data.access_token);
                            file.retry();
                        });
                    }
                    window.location.href = '/login';
                }).catch(function() {
                    window.location.href = '/login';
                });
                return;
            }
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
            stopUploadRefresh();
            updateProgressBall(100);
            hideProgressBall(1500);
            setTimeout(function() { window.location.reload(); }, 2000);
        });
    }

    // ========== 创建文件夹 ==========
    var createFolderForm = document.getElementById('create-folder-form');
    if (createFolderForm) {
        createFolderForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            var name = document.getElementById('folder-name').value;
            var parentInput = document.querySelector('#create-folder-form input[name="parent_id"]');
            var body = { name: name };
            if (parentInput && parentInput.value) body.parent_id = parseInt(parentInput.value);

            try {
                var response = await authFetch('/api/folders', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                var data = await response.json();
                if (response.ok) {
                    showMessage('文件夹创建成功');
                    hideCreateFolderModal();
                    setTimeout(function() { window.location.reload(); }, 1000);
                } else {
                    showMessage(data.error || '创建失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }

    // ========== 重命名 ==========
    var renameForm = document.getElementById('rename-form');
    if (renameForm) {
        renameForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            var id = document.getElementById('rename-item-id').value;
            var newName = document.getElementById('rename-item-name').value;

            try {
                var response = await authFetch('/api/files/' + id + '/rename', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: newName }),
                });
                var data = await response.json();
                if (response.ok) {
                    showMessage('重命名成功');
                    hideRenameModal();
                    setTimeout(function() { window.location.reload(); }, 1000);
                } else {
                    showMessage(data.error || '重命名失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }

    // ========== 移动 ==========
    var moveForm = document.getElementById('move-form');
    if (moveForm) {
        moveForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            var id = document.getElementById('move-item-id').value;
            var targetValue = document.getElementById('move-target-id').value;
            var body = {};
            if (targetValue) body.target_id = parseInt(targetValue);

            try {
                var response = await authFetch('/api/files/' + id + '/move', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                var data = await response.json();
                if (response.ok) {
                    showMessage('移动成功');
                    hideMoveModal();
                    setTimeout(function() { window.location.reload(); }, 1000);
                } else {
                    showMessage(data.error || '移动失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }

    // ========== 分享 ==========
    var shareForm = document.getElementById('share-form');
    if (shareForm) {
        shareForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            var itemId = document.getElementById('share-item-id').value;
            var itemType = document.getElementById('share-item-type').value;
            var projectName = document.getElementById('share-project-name').value;
            var password = document.getElementById('share-password').value;
            var expiresIn = document.getElementById('share-expires').value;

            var body = {
                item_id: parseInt(itemId),
                is_folder: itemType === 'folder',
                project_name: projectName,
                expires_in: expiresIn
            };
            if (password) body.password = password;

            try {
                var response = await authFetch('/api/shares', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                var data = await response.json();
                if (response.ok) {
                    document.getElementById('share-form-view').classList.add('hidden');
                    document.getElementById('share-success-view').classList.remove('hidden');
                    document.getElementById('share-link').value = window.location.origin + '/s/' + data.share_token;
                } else {
                    showMessage(data.error || '创建分享失败', true);
                }
            } catch (err) {
                showMessage('网络错误，请重试', true);
            }
        });
    }
});

// ========== 模态框操作 ==========

function showCreateFolderModal() {
    var modal = document.getElementById('create-folder-modal');
    if (modal) modal.classList.remove('hidden');
}

function hideCreateFolderModal() {
    var modal = document.getElementById('create-folder-modal');
    var input = document.getElementById('folder-name');
    if (modal) modal.classList.add('hidden');
    if (input) input.value = '';
}

function showRenameModal(id, currentName) {
    var modal = document.getElementById('rename-modal');
    var idInput = document.getElementById('rename-item-id');
    var nameInput = document.getElementById('rename-item-name');
    if (modal && idInput && nameInput) {
        idInput.value = id;
        nameInput.value = currentName;
        modal.classList.remove('hidden');
        nameInput.focus();
        nameInput.select();
    }
    document.querySelectorAll('[id^="dropdown-"]').forEach(function(el) { el.classList.add('hidden'); });
}

function showRenameModalFromLink(element) {
    showRenameModal(element.getAttribute('data-id'), element.getAttribute('data-name'));
}

function hideRenameModal() {
    var modal = document.getElementById('rename-modal');
    if (modal) modal.classList.add('hidden');
}

function showMoveModal(element) {
    var id = element.getAttribute('data-id');
    var modal = document.getElementById('move-modal');
    var idInput = document.getElementById('move-item-id');
    var select = document.getElementById('move-target-id');

    if (modal && idInput && select) {
        idInput.value = id;
        document.querySelectorAll('[id^="dropdown-"]').forEach(function(el) { el.classList.add('hidden'); });
        select.innerHTML = '<option value="">根目录</option>';

        authFetch('/api/folders/all').then(function(response) { return response.json(); }).then(function(data) {
            if (data.folders) {
                var folderMap = {};
                data.folders.forEach(function(f) { folderMap[f.ID] = f; });
                data.folders.forEach(function(f) {
                    if (String(f.ID) === String(id)) return;
                    var path = buildFolderPath(f, folderMap);
                    var option = document.createElement('option');
                    option.value = f.ID;
                    option.textContent = path;
                    select.appendChild(option);
                });
            }
        }).catch(function() {});

        modal.classList.remove('hidden');
    }
}

function hideMoveModal() {
    var modal = document.getElementById('move-modal');
    if (modal) modal.classList.add('hidden');
}

// ========== 分享 ==========

function showShareModal(element) {
    var id = element.getAttribute('data-id');
    var name = element.getAttribute('data-name');
    var type = element.getAttribute('data-type');
    var modal = document.getElementById('share-modal');
    var idInput = document.getElementById('share-item-id');
    var typeInput = document.getElementById('share-item-type');
    var nameInput = document.getElementById('share-project-name');

    if (modal && idInput && typeInput && nameInput) {
        idInput.value = id;
        typeInput.value = type;
        nameInput.value = name;
        document.getElementById('share-form-view').classList.remove('hidden');
        document.getElementById('share-success-view').classList.add('hidden');
        document.querySelectorAll('[id^="dropdown-"]').forEach(function(el) { el.classList.add('hidden'); });
        modal.classList.remove('hidden');
    }
}

function hideShareModal() {
    var modal = document.getElementById('share-modal');
    if (modal) modal.classList.add('hidden');
    document.getElementById('share-form-view').classList.remove('hidden');
    document.getElementById('share-success-view').classList.add('hidden');
    var passwordInput = document.getElementById('share-password');
    if (passwordInput) passwordInput.value = '';
    var expiresSelect = document.getElementById('share-expires');
    if (expiresSelect) expiresSelect.value = '7d';
}

function copyShareLink() {
    var linkInput = document.getElementById('share-link');
    if (linkInput) {
        linkInput.select();
        navigator.clipboard.writeText(linkInput.value).then(function() {
            showMessage('链接已复制到剪贴板');
        }).catch(function() {
            document.execCommand('copy');
            showMessage('链接已复制到剪贴板');
        });
    }
}

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

// ========== 删除 ==========

async function deleteItem(id, isFolder) {
    if (!confirm('确定要删除吗？')) return;

    try {
        var response = await authFetch('/api/files/' + id, { method: 'DELETE' });
        var data = await response.json();
        if (response.ok) {
            showMessage('删除成功');
            setTimeout(function() { window.location.reload(); }, 1000);
        } else {
            showMessage(data.error || '删除失败', true);
        }
    } catch (err) {
        showMessage('网络错误，请重试', true);
    }
}

// ========== 下载（通过短链接） ==========

async function downloadFile(id) {
    try {
        var response = await authFetch('/api/files/' + id + '/download-link', { method: 'POST' });
        var data = await response.json();
        if (response.ok) {
            window.location.href = data.url;
        } else {
            showMessage(data.error || '获取下载链接失败', true);
        }
    } catch (err) {
        showMessage('网络错误，请重试', true);
    }
}

async function downloadFolder(id) {
    if (!confirm('下载文件夹可能需要压缩过程，这会消耗一段时间，请您坐和放宽，下载马上开始。确认下载吗？')) return;

    try {
        var response = await authFetch('/api/folders/' + id + '/download-link', { method: 'POST' });
        var data = await response.json();
        if (response.ok) {
            window.location.href = data.url;
        } else {
            showMessage(data.error || '获取下载链接失败', true);
        }
    } catch (err) {
        showMessage('网络错误，请重试', true);
    }
}

// ========== 退出登录 ==========

function logout() {
    fetch('/api/auth/logout', { method: 'POST' }).finally(function() {
        window.location.href = '/login';
    });
}

// ========== 下拉菜单 ==========

function toggleDropdown(id) {
    document.querySelectorAll('[id^="dropdown-"]').forEach(function(el) {
        if (el.id !== id) el.classList.add('hidden');
    });
    var dropdown = document.getElementById(id);
    if (dropdown) dropdown.classList.toggle('hidden');
}

document.addEventListener('click', function(e) {
    if (!e.target.closest('[onclick^="toggleDropdown"]') && !e.target.closest('[id^="dropdown-"]')) {
        document.querySelectorAll('[id^="dropdown-"]').forEach(function(el) { el.classList.add('hidden'); });
    }
});

// ========== 文件预览 ==========

var PREVIEW_MAX_SIZE = 50 * 1024 * 1024; // 50MB

var PREVIEWABLE_EXTS = {
    image: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico', 'tiff', 'tif'],
    video: ['mp4', 'webm', 'ogg', 'ogv'],
    audio: ['mp3', 'wav', 'ogg', 'oga', 'm4a', 'flac', 'aac'],
    pdf: ['pdf'],
    text: [
        'txt', 'log', 'md', 'markdown', 'go', 'py', 'java', 'c', 'cpp', 'h', 'hpp',
        'rs', 'rb', 'php', 'sh', 'bash', 'zsh', 'js', 'ts', 'tsx', 'jsx', 'vue',
        'css', 'scss', 'less', 'html', 'htm', 'xml', 'json', 'yaml', 'yml',
        'toml', 'ini', 'cfg', 'conf', 'sql', 'swift', 'kt', 'scala', 'dart',
        'lua', 'pl', 'r', 'makefile', 'dockerfile', 'gitignore', 'env',
        'csv', 'bat', 'ps1', 'gradle', 'cmake', 'properties'
    ],
    markdown: ['md', 'markdown']
};

function getFileCategory(name) {
    var ext = name.split('.').pop().toLowerCase();
    for (var cat in PREVIEWABLE_EXTS) {
        if (PREVIEWABLE_EXTS[cat].indexOf(ext) !== -1) {
            return cat;
        }
    }
    return null;
}

function previewFile(element) {
    var id = element.getAttribute('data-id');
    var name = element.getAttribute('data-name');
    var size = parseInt(element.getAttribute('data-size'), 10);

    // 关闭下拉菜单
    document.querySelectorAll('[id^="dropdown-"]').forEach(function(el) { el.classList.add('hidden'); });

    var category = getFileCategory(name);
    if (!category) {
        showMessage('该文件格式不支持在线预览', true);
        return;
    }

    if (size > PREVIEW_MAX_SIZE) {
        showMessage('文件过大（超过 50MB），请下载查看', true);
        return;
    }

    // 创建下载链接用于预览
    authFetch('/api/files/' + id + '/download-link', { method: 'POST' })
        .then(function(response) { return response.json(); })
        .then(function(data) {
            if (data.url) {
                var previewUrl = data.url + '?inline=1';
                showPreviewModal(name, previewUrl, category);
            } else {
                showMessage(data.error || '获取预览链接失败', true);
            }
        })
        .catch(function() {
            showMessage('网络错误，请重试', true);
        });
}

function showPreviewModal(name, url, category) {
    var modal = document.getElementById('preview-modal');
    var title = document.getElementById('preview-title');
    var content = document.getElementById('preview-content');

    title.textContent = name;
    content.innerHTML = '';

    if (category === 'image') {
        var img = document.createElement('img');
        img.src = url;
        img.alt = name;
        img.style.maxWidth = '100%';
        img.style.maxHeight = '100%';
        img.style.objectFit = 'contain';
        content.appendChild(img);
    } else if (category === 'video') {
        var video = document.createElement('video');
        video.src = url;
        video.controls = true;
        video.style.maxWidth = '100%';
        video.style.maxHeight = '100%';
        content.appendChild(video);
    } else if (category === 'audio') {
        var audioContainer = document.createElement('div');
        audioContainer.className = 'text-center';
        var audioIcon = document.createElement('div');
        audioIcon.innerHTML = '<svg class="mx-auto mb-4" style="width:80px;height:80px;color:#6b7280" fill="currentColor" viewBox="0 0 24 24"><path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55C7.79 13 6 14.79 6 17s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z"/></svg>';
        audioContainer.appendChild(audioIcon);
        var audio = document.createElement('audio');
        audio.src = url;
        audio.controls = true;
        audioContainer.appendChild(audio);
        content.appendChild(audioContainer);
    } else if (category === 'pdf') {
        var iframe = document.createElement('iframe');
        iframe.src = url;
        iframe.style.width = '100%';
        iframe.style.height = '100%';
        iframe.style.border = 'none';
        content.appendChild(iframe);
    } else if (category === 'markdown') {
        content.style.justifyContent = 'flex-start';
        content.style.alignItems = 'flex-start';
        fetch(url).then(function(r) { return r.text(); }).then(function(text) {
            var div = document.createElement('div');
            div.className = 'prose max-w-none w-full';
            div.innerHTML = marked.parse(text);
            content.appendChild(div);
        }).catch(function() {
            content.innerHTML = '<p class="text-red-500">加载文件内容失败</p>';
        });
    } else if (category === 'text') {
        content.style.justifyContent = 'flex-start';
        content.style.alignItems = 'flex-start';
        fetch(url).then(function(r) { return r.text(); }).then(function(text) {
            var ext = name.split('.').pop().toLowerCase();
            // highlight.js 语言映射
            var langMap = {
                'go': 'go', 'py': 'python', 'java': 'java', 'c': 'c', 'cpp': 'cpp',
                'h': 'c', 'hpp': 'cpp', 'rs': 'rust', 'rb': 'ruby', 'php': 'php',
                'sh': 'bash', 'bash': 'bash', 'zsh': 'bash', 'js': 'javascript',
                'ts': 'typescript', 'tsx': 'typescript', 'jsx': 'javascript',
                'vue': 'html', 'css': 'css', 'scss': 'scss', 'less': 'less',
                'html': 'html', 'htm': 'html', 'xml': 'xml', 'json': 'json',
                'yaml': 'yaml', 'yml': 'yaml', 'toml': 'ini', 'ini': 'ini',
                'sql': 'sql', 'swift': 'swift', 'kt': 'kotlin', 'scala': 'scala',
                'dart': 'dart', 'lua': 'lua', 'pl': 'perl', 'r': 'r',
                'md': 'markdown', 'csv': 'plaintext', 'log': 'plaintext',
                'txt': 'plaintext', 'conf': 'nginx', 'cfg': 'ini'
            };
            var lang = langMap[ext] || 'plaintext';
            var pre = document.createElement('pre');
            pre.style.width = '100%';
            pre.style.margin = '0';
            var code = document.createElement('code');
            code.className = 'language-' + lang;
            code.textContent = text;
            pre.appendChild(code);
            hljs.highlightElement(code);
            content.appendChild(pre);
        }).catch(function() {
            content.innerHTML = '<p class="text-red-500">加载文件内容失败</p>';
        });
    }

    modal.classList.remove('hidden');
}

function hidePreviewModal() {
    var modal = document.getElementById('preview-modal');
    var content = document.getElementById('preview-content');
    if (modal) modal.classList.add('hidden');
    // 清理资源：停止视频/音频播放
    if (content) {
        var media = content.querySelector('video, audio');
        if (media) media.pause();
        content.innerHTML = '';
        content.style.justifyContent = '';
        content.style.alignItems = '';
    }
}

function closePreview(event) {
    if (event.target === document.getElementById('preview-modal')) {
        hidePreviewModal();
    }
}

// ESC 关闭预览
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        var modal = document.getElementById('preview-modal');
        if (modal && !modal.classList.contains('hidden')) {
            hidePreviewModal();
        }
    }
});
