// 全局变量
let currentPath = '.';
let directoryTree = {};
let fileList = [];
let uploadModal = null;
let createDirModal = null;
let createSymlinkModal = null;
let apiBasePath = ''; // API基础路径

// 初始化函数
$(document).ready(function() {
    console.log('文件管理器初始化开始...');
    
    // 检测当前访问路径，自动设置API基础路径
    detectApiBasePath();
    console.log('检测到的API基础路径:', apiBasePath);
    
    // 初始化模态框
    uploadModal = new bootstrap.Modal(document.getElementById('upload-modal'));
    createDirModal = new bootstrap.Modal(document.getElementById('create-dir-modal'));
    createSymlinkModal = new bootstrap.Modal(document.getElementById('create-symlink-modal'));

    // 延迟加载数据，确保DOM完全就绪
    setTimeout(function() {
        console.log('开始加载数据...');
        // 加载目录树和文件列表
        loadDirectoryTree();
        loadFileList(currentPath);

        // 绑定事件
        bindEvents();
        
        console.log('文件管理器初始化完成');
    }, 100);
});

// 检测API基础路径
function detectApiBasePath() {
    // 检查当前URL是否包含/filesuploader路径
    if (window.location.pathname.includes('/filesuploader')) {
        // 如果是通过反向代理访问，设置基础路径
        apiBasePath = '/filesuploader';
    } else {
        // 如果是直接访问6012端口，使用根路径
        apiBasePath = '';
    }
    
    // 确保基础路径以/结尾
    if (apiBasePath && !apiBasePath.endsWith('/')) {
        apiBasePath += '/';
    }
    
    console.log('API基础路径设置为:', apiBasePath);
    console.log('当前URL:', window.location.href);
    console.log('当前路径:', window.location.pathname);
    
    // 确保静态文件路径正确
    ensureStaticFilePaths();
}

// 确保静态文件路径正确
function ensureStaticFilePaths() {
    console.log('确保静态文件路径正确...');
    
    // 检查并修正CSS文件路径
    document.querySelectorAll('link[rel="stylesheet"]').forEach(link => {
        let href = link.getAttribute('href');
        if (href && href.startsWith('/static/')) {
            // 如果当前路径包含/filesuploader，但CSS路径是绝对路径，需要修正
            if (window.location.pathname.includes('/filesuploader') && !href.startsWith('/filesuploader/')) {
                let newHref = '/filesuploader' + href;
                console.log('修正CSS路径:', href, '->', newHref);
                link.setAttribute('href', newHref);
            }
        }
    });
    
    // 检查并修正JavaScript文件路径
    document.querySelectorAll('script[src]').forEach(script => {
        let src = script.getAttribute('src');
        if (src && src.startsWith('/static/')) {
            // 如果当前路径包含/filesuploader，但JS路径是绝对路径，需要修正
            if (window.location.pathname.includes('/filesuploader') && !src.startsWith('/filesuploader/')) {
                let newSrc = '/filesuploader' + src;
                console.log('修正JS路径:', src, '->', newSrc);
                script.setAttribute('src', newSrc);
            }
        }
    });
    
    console.log('静态文件路径修正完成');
}

// 绑定事件
function bindEvents() {
    console.log('绑定事件开始...');
    
    // 上传文件按钮
    $('#btn-upload').on('click', function() {
        console.log('上传按钮点击');
        $('#upload-path').val(currentPath);
        uploadModal.show();
    });

    // 创建目录按钮
    $('#btn-create-dir').on('click', function() {
        $('#create-dir-path').val(currentPath);
        createDirModal.show();
    });

    // 创建软链接按钮
    $('#btn-create-symlink').on('click', function() {
        $('#create-symlink-path').val(currentPath);
        createSymlinkModal.show();
    });

    // 搜索按钮
    $('#btn-search').on('click', function() {
        searchFiles($('#search-input').val());
    });

    // 搜索输入框回车
    $('#search-input').on('keypress', function(e) {
        if (e.which === 13) {
            searchFiles($(this).val());
        }
    });

    // 上传表单提交
    $('#btn-submit-upload').on('click', function() {
        console.log('上传表单提交按钮点击');
        let input = document.getElementById('file-input');
        if (input.files.length === 0) {
            showToast('请选择要上传的文件', 'warning');
            return;
        }
        
        console.log('选择的文件数量:', input.files.length);
        for (let i = 0; i < input.files.length; i++) {
            console.log('文件', i + 1, ':', input.files[i].name, '(', input.files[i].size, 'bytes)');
        }
        
        // 执行实际的上传操作
        uploadFilesFromInput(input);
    });

    // 创建目录表单提交
    $('#btn-submit-create-dir').on('click', function() {
        createDirectory();
    });

    // 创建软链接表单提交
    $('#btn-submit-create-symlink').on('click', function() {
        createSymlink();
    });

    // 拖拽上传
    $(document).on('dragover', function(e) {
        e.preventDefault();
        e.stopPropagation();
    });

    $(document).on('drop', function(e) {
        e.preventDefault();
        e.stopPropagation();
    });

    $('#file-list').on('dragover', function(e) {
        e.preventDefault();
        e.stopPropagation();
        $(this).addClass('dropzone active');
    });

    $('#file-list').on('dragleave', function(e) {
        e.preventDefault();
        e.stopPropagation();
        $(this).removeClass('active');
    });

    $('#file-list').on('drop', function(e) {
        e.preventDefault();
        e.stopPropagation();
        $(this).removeClass('active');

        if (e.originalEvent.dataTransfer.files.length > 0) {
            console.log('拖拽上传文件数量:', e.originalEvent.dataTransfer.files.length);
            $('#hidden-file-input')[0].files = e.originalEvent.dataTransfer.files;
            $('#upload-path').val(currentPath);
            uploadFilesFromInput($('#hidden-file-input')[0]);
        }
    });
    
    console.log('事件绑定完成');
}

// 加载目录树
function loadDirectoryTree() {
    console.log('开始加载目录树...');
    let apiUrl = apiBasePath + 'api/directory/tree';
    console.log('目录树API URL:', apiUrl);
    console.log('完整请求URL:', window.location.origin + apiUrl);
    
    // 确保API基础路径正确
    if (!apiUrl.startsWith('http')) {
        if (!apiUrl.startsWith('/')) {
            apiUrl = '/' + apiUrl;
        }
    }
    
    console.log('最终API URL:', apiUrl);
    
    $.ajax({
        url: apiUrl,
        type: 'GET',
        dataType: 'json',
        success: function(response) {
            console.log('目录树API响应:', response);
            if (response.files && Array.isArray(response.files)) {
                console.log('获取到文件数量:', response.files.length);
                try {
                    // 构建目录树数据结构
                    directoryTree = buildDirectoryTree(response.files);
                    console.log('构建的目录树:', directoryTree);
                    // 渲染目录树
                    renderDirectoryTree(directoryTree, $('#directory-tree'));
                } catch (err) {
                    console.error('构建或渲染目录树时出错:', err);
                    showToast('目录树处理失败: ' + err.message, 'error');
                    $('#directory-tree').html('<div class="text-center text-danger py-5"><i class="fa fa-exclamation-triangle fa-2x"></i><p class="mt-2">处理失败</p></div>');
                }
            } else {
                console.error('目录树数据格式错误:', response);
                showToast('目录树数据格式错误', 'error');
                $('#directory-tree').html('<div class="text-center text-danger py-5"><i class="fa fa-exclamation-triangle fa-2x"></i><p class="mt-2">数据格式错误</p></div>');
            }
        },
        error: function(xhr, status, error) {
            console.error('加载目录树失败:', error, xhr.responseText);
            showToast('加载目录树失败: ' + error, 'error');
            $('#directory-tree').html('<div class="text-center text-danger py-5"><i class="fa fa-exclamation-triangle fa-2x"></i><p class="mt-2">加载失败</p></div>');
        }
    });
}

// 构建目录树数据结构
function buildDirectoryTree(files) {
    let tree = { name: '/', path: '.', children: {} };

    console.log('目录树构建前文件数量:', files.length);
    let treeFiles = files.filter(file => {
        let fileName = file.name || '';
        let shouldHide = fileName.toLowerCase().startsWith('_h5ai');
        if (shouldHide) {
            console.log('目录树构建时隐藏文件:', fileName, '原始名称:', file.name);
        }
        return !shouldHide && (file.isDir || file.isSymlink);
    });
    console.log('目录树构建使用文件数量:', treeFiles.length);
    
    treeFiles.forEach(file => {
        // 使用小写的字段名匹配Go的JSON输出
        if (file.isDir || file.isSymlink) {
            let pathParts = (file.path || '').split('/');
            let current = tree;

            for (let i = 0; i < pathParts.length; i++) {
                let part = pathParts[i];
                if (part === '') continue;

                if (!current.children[part]) {
                    current.children[part] = {
                        name: part,
                        path: pathParts.slice(0, i + 1).join('/'),
                        isSymlink: file.isSymlink,
                        symlinkTarget: file.symlinkTarget,
                        children: {}
                    };
                }

                current = current.children[part];
            }
        }
    });

    return tree;
}

// 渲染目录树
function renderDirectoryTree(node, container) {
    container.empty();

    // 递归渲染函数
    function renderNode(node, parentElement, level = 0) {
        let itemElement = $('<div class="tree-item"></div>');
        
        // 添加切换按钮（如果有子节点）
        if (Object.keys(node.children).length > 0) {
            let toggleElement = $('<span class="toggle"><i class="fa fa-caret-right"></i></span>');
            toggleElement.on('click', function(e) {
                e.stopPropagation();
                let childrenElement = $(this).parent().find('.tree-children');
                let icon = $(this).find('i');
                
                if (childrenElement.is(':visible')) {
                    childrenElement.hide();
                    icon.removeClass('fa-caret-down').addClass('fa-caret-right');
                } else {
                    childrenElement.show();
                    icon.removeClass('fa-caret-right').addClass('fa-caret-down');
                }
            });
            itemElement.append(toggleElement);
        } else {
            itemElement.append('<span class="toggle"></span>');
        }

        // 添加图标
        let iconClass = node.isSymlink ? 'fa-link text-info' : 'fa-folder text-warning';
        itemElement.append(`<span class="icon"><i class="fa ${iconClass}"></i></span>`);

        // 添加名称
        let nameElement = $('<span class="name"></span>').text(node.name);
        itemElement.append(nameElement);

        // 点击事件
        itemElement.on('click', function(e) {
            // 如果点击的是切换按钮，不执行目录切换
            if ($(e.target).closest('.toggle').length > 0) {
                return;
            }
            
            console.log('目录树项点击:', node.name, node.path);
            
            // 移除其他项的活动状态
            $('.tree-item').removeClass('active');
            // 添加当前项的活动状态
            $(this).addClass('active');
            // 加载文件列表
            loadFileList(node.path);
            // 更新当前路径
            currentPath = node.path;
            // 更新当前路径显示
            updateCurrentPathDisplay();
        });

        // 添加到父元素
        parentElement.append(itemElement);

        // 如果有子节点，递归渲染
        if (Object.keys(node.children).length > 0) {
            let childrenElement = $('<div class="tree-children"></div>');
            if (level > 0) {
                childrenElement.hide();
            }
            
            // 按名称排序
            let sortedKeys = Object.keys(node.children).sort();
            
            sortedKeys.forEach(key => {
                renderNode(node.children[key], childrenElement, level + 1);
            });
            
            parentElement.append(childrenElement);
        }
    }

    // 从根节点开始渲染
    renderNode(node, container);

    // 默认展开根节点
    if (Object.keys(node.children).length > 0) {
        container.find('.tree-item:first .toggle i').removeClass('fa-caret-right').addClass('fa-caret-down');
        container.find('.tree-children:first').show();
    }
}

// 加载文件列表
function loadFileList(path) {
    console.log('开始加载文件列表，路径:', path);
    let apiUrl = apiBasePath + `api/directory/list/${path}`;
    console.log('目录树API URL:', apiUrl);
    console.log('完整请求URL:', window.location.origin + apiUrl);
    
    // 确保API基础路径正确
    if (!apiUrl.startsWith('http')) {
        if (!apiUrl.startsWith('/')) {
            apiUrl = '/' + apiUrl;
        }
    }
    
    console.log('最终API URL:', apiUrl);
    
    $.ajax({
        url: apiUrl,
        type: 'GET',
        dataType: 'json',
        success: function(response) {
            console.log('文件列表API响应:', response);
            if (response.files && Array.isArray(response.files)) {
                console.log('获取到文件数量:', response.files.length);
                try {
                    fileList = response.files;
                    renderFileList(fileList);
                    // 更新当前路径
                    currentPath = response.path || path;
                    console.log('更新当前路径:', currentPath);
                    // 更新当前路径显示
                    updateCurrentPathDisplay();
                    // 更新目录树活动状态
                    updateDirectoryTreeActive(currentPath);
                } catch (err) {
                    console.error('处理文件列表时出错:', err);
                    showToast('文件列表处理失败: ' + err.message, 'error');
                    $('#file-list').html('<div class="text-center text-danger py-5"><i class="fa fa-exclamation-triangle fa-2x"></i><p class="mt-2">处理失败</p></div>');
                }
            } else {
                console.error('文件列表数据格式错误:', response);
                showToast('文件列表数据格式错误', 'error');
                $('#file-list').html('<div class="text-center text-danger py-5"><i class="fa fa-exclamation-triangle fa-2x"></i><p class="mt-2">数据格式错误</p></div>');
            }
        },
        error: function(xhr, status, error) {
            console.error('加载文件列表失败 - 状态:', status);
            console.error('加载文件列表失败 - 错误:', error);
            console.error('加载文件列表失败 - HTTP状态码:', xhr.status);
            console.error('加载文件列表失败 - 响应文本:', xhr.responseText);
            console.error('加载文件列表失败 - 响应头:', xhr.getAllResponseHeaders());
            
            let errorMsg = '加载文件列表失败';
            if (xhr.status === 404) {
                errorMsg += '：请求的资源不存在 (404)';
            } else if (xhr.status === 500) {
                errorMsg += '：服务器内部错误 (500)';
            } else if (xhr.status === 0) {
                errorMsg += '：无法连接到服务器，请检查网络连接';
            } else {
                errorMsg += '：' + error;
            }
            
            showToast(errorMsg, 'error');
            $('#file-list').html('<div class="text-center text-danger py-5"><i class="fa fa-exclamation-triangle fa-2x"></i><p class="mt-2">加载失败</p><p class="text-sm">状态码: ' + xhr.status + '</p></div>');
        }
    });
}

// 渲染文件列表
function renderFileList(files) {
    let container = $('#file-list');
    container.empty();

    // 过滤掉以_h5ai开头的文件和文件夹（更严格的检查）
    console.log('过滤前文件数量:', files.length);
    console.log('原始文件列表:', JSON.stringify(files.map(f => f.name), null, 2));
    
    let filteredFiles = files.filter(file => {
        let fileName = file.name || '';
        let shouldHide = fileName.toLowerCase().startsWith('_h5ai');
        if (shouldHide) {
            console.log('文件列表中隐藏文件:', fileName, '原始名称:', file.name);
        }
        return !shouldHide;
    });
    
    console.log('过滤后文件数量:', filteredFiles.length);
    console.log('过滤后文件列表:', JSON.stringify(filteredFiles.map(f => f.name), null, 2));
    
    if (filteredFiles.length === 0) {
        container.html('<div class="text-center text-muted py-5"><i class="fa fa-folder-open-o fa-2x"></i><p class="mt-2">目录为空</p></div>');
        return;
    }

    // 按类型和名称排序（目录在前，文件在后）
    filteredFiles.sort((a, b) => {
        if (a.isDir && !b.isDir) return -1;
        if (!a.isDir && b.isDir) return 1;
        return (a.name || '').localeCompare(b.name || '');
    });

    filteredFiles.forEach(file => {
        let card = $('<div class="col"></div>');
        let cardInner = $('<div class="file-card card h-100"></div>');
        // 确定图标
        let iconClass = 'fa-file-o';
        let iconColor = 'text-muted';
        
        if (file.isDir) {
            iconClass = 'fa-folder';
            iconColor = 'text-warning';
        } else if (file.isSymlink) {
            iconClass = 'fa-link';
            iconColor = 'text-info';
        } else {
            // 根据文件扩展名设置图标
            let ext = (file.name || '').split('.').pop().toLowerCase();
            switch (ext) {
                case 'txt': iconClass = 'fa-file-text-o'; break;
                case 'pdf': iconClass = 'fa-file-pdf-o'; iconColor = 'text-danger'; break;
                case 'doc':
                case 'docx': iconClass = 'fa-file-word-o'; iconColor = 'text-blue'; break;
                case 'xls':
                case 'xlsx': iconClass = 'fa-file-excel-o'; iconColor = 'text-green'; break;
                case 'ppt':
                case 'pptx': iconClass = 'fa-file-powerpoint-o'; iconColor = 'text-orange'; break;
                case 'jpg':
                case 'jpeg':
                case 'png':
                case 'gif': iconClass = 'fa-file-image-o'; iconColor = 'text-purple'; break;
                case 'zip':
                case 'rar':
                case 'tar':
                case 'gz': iconClass = 'fa-file-archive-o'; iconColor = 'text-red'; break;
                case 'js':
                case 'css':
                case 'html':
                case 'php':
                case 'go':
                case 'py':
                case 'java':
                case 'c':
                case 'cpp':
                case 'h':
                case 'hpp': iconClass = 'fa-file-code-o'; iconColor = 'text-info'; break;
                case 'mp3':
                case 'wav':
                case 'ogg': iconClass = 'fa-file-audio-o'; iconColor = 'text-green'; break;
                case 'mp4':
                case 'avi':
                case 'mov':
                case 'mkv': iconClass = 'fa-file-video-o'; iconColor = 'text-red'; break;
            }
        }

        // 格式化文件大小
        let fileSize = formatFileSize(file.size || 0);
        
        // 格式化修改时间
        let modTime = new Date((file.modTime || 0) * 1000).toLocaleString();

        // 构建卡片内容
        cardInner.append(`
            <div class="card-body">
                <div class="file-icon ${iconColor}">
                    <i class="fa ${iconClass}"></i>
                </div>
                <div class="file-name" title="${file.name || ''}">${file.name || '未知文件'}</div>
                <div class="file-info mt-auto">
                    <div class="d-flex justify-content-between">
                        <span>${fileSize}</span>
                        <span>${modTime}</span>
                    </div>
                    ${file.isSymlink ? `<div class="text-xs text-info mt-1">链接到: ${file.symlinkTarget || ''}</div>` : ''}
                </div>
            </div>
        `);

        // 添加点击事件
        if (file.isDir || file.isSymlink) {
            cardInner.css('cursor', 'pointer');
            // 单击事件保持（可选：可以移除或保留）
            cardInner.on('click', function() {
                // 单击可以用于选择或其他操作
            });
            // 双击事件用于进入目录
            cardInner.on('dblclick', function() {
                console.log('双击进入目录:', file.path);
                loadFileList(file.path);
            });
        }
        
        // 确保_h5ai文件不会显示（额外的安全检查）
        let fileName = file.name || '';
        if (fileName.toLowerCase().startsWith('_h5ai')) {
            console.log('发现_h5ai文件，应该已经被过滤:', fileName);
            cardInner.hide();
        }

        // 添加右键菜单
        cardInner.on('contextmenu', function(e) {
            e.preventDefault();
            showContextMenu(e, file);
        });

        card.append(cardInner);
        container.append(card);
    });
}

// 更新当前路径显示
function updateCurrentPathDisplay() {
    let displayPath = currentPath === '.' ? '/' : '/' + currentPath;
    $('#current-path').text('当前路径: ' + displayPath);
}

// 更新目录树活动状态
function updateDirectoryTreeActive(path) {
    // 移除所有活动状态
    $('.tree-item').removeClass('active');
    
    // 如果是根目录
    if (path === '.') {
        $('#directory-tree .tree-item:first').addClass('active');
        return;
    }
    
    // 查找并激活对应节点
    let pathParts = path.split('/');
    let current = directoryTree;
    let found = true;
    
    for (let i = 0; i < pathParts.length; i++) {
        let part = pathParts[i];
        if (part === '') continue;
        
        if (current.children && current.children[part]) {
            current = current.children[part];
        } else {
            found = false;
            break;
        }
    }
    
    if (found) {
        // 查找对应的DOM元素
        let selector = '.tree-item';
        pathParts.forEach(part => {
            if (part !== '') {
                selector += `:has(.name:contains('${part}'))`;
            }
        });
        
        // 激活找到的元素
        $(selector).addClass('active');
        
        // 展开所有父节点
        $(selector).parents('.tree-children').show();
        $(selector).parents('.tree-item').find('.toggle i').removeClass('fa-caret-right').addClass('fa-caret-down');
    }
}

// 上传文件
function uploadFiles() {
    let input = document.getElementById('file-input');
    if (input.files.length === 0) {
        showToast('请选择要上传的文件', 'warning');
        return;
    }
    
    // 设置当前路径到上传表单
    $('#upload-path').val(currentPath);
    console.log('上传到路径:', currentPath);
    
    // 显示上传模态框，让用户确认或修改上传路径
    uploadModal.show();
    
    // 上传操作在模态框的提交按钮中处理
}

// 从输入框上传文件
function uploadFilesFromInput(input) {
    console.log('开始上传文件...');
    let path = $('#upload-path').val();
    console.log('上传路径:', path);
    
    let formData = new FormData();
    
    // 添加路径
    formData.append('path', path);
    console.log('FormData添加路径参数');
    
    // 添加文件
    for (let i = 0; i < input.files.length; i++) {
        formData.append('files', input.files[i]);
        console.log('FormData添加文件:', i + 1, input.files[i].name);
    }
    
    // 显示进度条
    $('#upload-progress').removeClass('d-none');
    console.log('显示上传进度条');
    
    // 使用Axios上传
    let apiUrl = apiBasePath + 'api/file/upload';
    console.log('开始发送POST请求到:', apiUrl);
    
    // 确保API基础路径正确
    if (!apiUrl.startsWith('http')) {
        if (!apiUrl.startsWith('/')) {
            apiUrl = '/' + apiUrl;
        }
    }
    
    console.log('最终API URL:', apiUrl);
    
    axios.post(apiUrl, formData, {
        headers: {
            'Content-Type': 'multipart/form-data'
        },
        onUploadProgress: function(progressEvent) {
            if (progressEvent.total) {
                let percentComplete = Math.round((progressEvent.loaded * 100) / progressEvent.total);
                $('#upload-progress-bar').css('width', percentComplete + '%');
                $('#upload-percentage').text(percentComplete + '%');
                
                // 显示当前上传的文件名
                if (input.files.length > 0) {
                    $('#upload-filename').text(input.files[0].name);
                }
            }
        }
    })
    .then(function(response) {
        console.log('上传请求成功，响应:', response.data);
        // 隐藏进度条
        $('#upload-progress').addClass('d-none');
        $('#upload-progress-bar').css('width', '0%');
        
        // 清空文件输入
        $('#file-input').val('');
        $('#hidden-file-input').val('');
        
        // 关闭模态框
        uploadModal.hide();
        
        // 显示结果
        if (response.data.success) {
            showToast('文件上传成功', 'success');
            console.log('上传成功，重新加载文件列表和目录树');
            // 重新加载文件列表（使用上传时的路径）
            loadFileList(path);
            // 重新加载目录树
            loadDirectoryTree();
        } else {
            let errorMsg = '文件上传失败';
            if (response.data.errors && response.data.errors.length > 0) {
                errorMsg += ': ' + response.data.errors.join(', ');
            }
            showToast(errorMsg, 'error');
        }
    })
    .catch(function(error) {
        console.error('上传请求失败:', error);
        // 隐藏进度条
        $('#upload-progress').addClass('d-none');
        $('#upload-progress-bar').css('width', '0%');
        
        // 清空文件输入
        $('#file-input').val('');
        $('#hidden-file-input').val('');
        
        // 关闭模态框
        uploadModal.hide();
        
        // 显示错误
        let errorMsg = '文件上传失败';
        if (error.response) {
            errorMsg += ': ' + error.response.status + ' ' + error.response.statusText;
            if (error.response.data && error.response.data.error) {
                errorMsg += ' - ' + error.response.data.error;
            }
        } else if (error.request) {
            errorMsg += ': 服务器无响应';
        } else {
            errorMsg += ': ' + error.message;
        }
        showToast(errorMsg, 'error');
    });
}

// 创建目录
function createDirectory() {
    let parentPath = $('#create-dir-path').val();
    let name = $('#dir-name').val().trim();
    
    if (!name) {
        showToast('请输入目录名称', 'warning');
        return;
    }
    
    let apiUrl = apiBasePath + 'api/directory/create';
    console.log('创建目录API URL:', apiUrl);
    
    // 确保API基础路径正确
    if (!apiUrl.startsWith('http')) {
        if (!apiUrl.startsWith('/')) {
            apiUrl = '/' + apiUrl;
        }
    }
    
    console.log('最终API URL:', apiUrl);
    
    $.ajax({
        url: apiUrl,
        type: 'POST',
        data: {
            parentPath: parentPath,
            name: name
        },
        dataType: 'json',
        success: function(response) {
            // 关闭模态框
            createDirModal.hide();
            // 清空输入
            $('#dir-name').val('');
            // 显示结果
            showToast('目录创建成功', 'success');
            // 重新加载文件列表
            loadFileList(currentPath);
            // 重新加载目录树
            loadDirectoryTree();
        },
        error: function(xhr, status, error) {
            let errorMsg = '目录创建失败';
            if (xhr.responseJSON && xhr.responseJSON.error) {
                errorMsg += ': ' + xhr.responseJSON.error;
            } else {
                errorMsg += ': ' + error;
            }
            showToast(errorMsg, 'error');
            console.error('创建目录失败:', xhr.responseJSON, error);
        }
    });
}

// 创建软链接
function createSymlink() {
    let parentPath = $('#create-symlink-path').val();
    let name = $('#symlink-name').val().trim();
    let target = $('#symlink-target').val().trim();
    
    if (!name) {
        showToast('请输入链接名称', 'warning');
        return;
    }
    
    if (!target) {
        showToast('请输入目标路径', 'warning');
        return;
    }
    
    let apiUrl = apiBasePath + 'api/directory/symlink';
    console.log('创建软链接API URL:', apiUrl);
    
    // 确保API基础路径正确
    if (!apiUrl.startsWith('http')) {
        if (!apiUrl.startsWith('/')) {
            apiUrl = '/' + apiUrl;
        }
    }
    
    console.log('最终API URL:', apiUrl);
    
    $.ajax({
        url: apiUrl,
        type: 'POST',
        data: {
            parentPath: parentPath,
            name: name,
            target: target
        },
        dataType: 'json',
        success: function(response) {
            // 关闭模态框
            createSymlinkModal.hide();
            // 清空输入
            $('#symlink-name').val('');
            $('#symlink-target').val('');
            // 显示结果
            showToast('软链接创建成功', 'success');
            // 重新加载文件列表
            loadFileList(currentPath);
            // 重新加载目录树
            loadDirectoryTree();
        },
        error: function(xhr, status, error) {
            let errorMsg = '软链接创建失败';
            if (xhr.responseJSON && xhr.responseJSON.error) {
                errorMsg += ': ' + xhr.responseJSON.error;
            } else {
                errorMsg += ': ' + error;
            }
            showToast(errorMsg, 'error');
        }
    });
}

// 搜索文件
function searchFiles(keyword) {
    if (!keyword.trim()) {
        renderFileList(fileList);
        return;
    }
    
    let filtered = fileList.filter(file => {
        return file.name.toLowerCase().includes(keyword.toLowerCase());
    });
    
    renderFileList(filtered);
}

// 显示右键菜单
function showContextMenu(e, file) {
    // 移除已存在的菜单
    $('.context-menu').remove();
    
    // 创建菜单
    let menu = $('<div class="context-menu dropdown-menu show"></div>');
    menu.css({
        position: 'absolute',
        left: e.pageX,
        top: e.pageY,
        zIndex: 1000
    });
    
    // 添加菜单项
    if (file.isDir || file.isSymlink) {
        menu.append(`
            <a class="dropdown-item" href="#" data-action="open">
                <i class="fa fa-folder-open mr-2"></i>打开
            </a>
        `);
    }
    
    menu.append(`
        <a class="dropdown-item" href="#" data-action="rename">
            <i class="fa fa-pencil mr-2"></i>重命名
        </a>
        <a class="dropdown-item text-danger" href="#" data-action="delete">
            <i class="fa fa-trash mr-2"></i>删除
        </a>
    `);
    
    // 添加到文档
    $('body').append(menu);
    
    // 绑定菜单项点击事件
    menu.find('a').on('click', function(e) {
        e.preventDefault();
        let action = $(this).data('action');
        
        switch (action) {
            case 'open':
                loadFileList(file.path);
                break;
            case 'rename':
                renameFile(file);
                break;
            case 'delete':
                deleteFile(file);
                break;
        }
        
        // 移除菜单
        menu.remove();
    });
    
    // 点击其他地方关闭菜单
    $(document).one('click', function() {
        menu.remove();
    });
}

// 重命名文件
function renameFile(file) {
    let newName = prompt('请输入新名称:', file.name || '未知文件');
    
    if (newName !== null && newName.trim() !== '') {
        let apiUrl = apiBasePath + 'api/file/rename';
        console.log('重命名文件API URL:', apiUrl);
        
        // 确保API基础路径正确
        if (!apiUrl.startsWith('http')) {
            if (!apiUrl.startsWith('/')) {
                apiUrl = '/' + apiUrl;
            }
        }
        
        console.log('最终API URL:', apiUrl);
        
        $.ajax({
            url: apiUrl,
            type: 'POST',
            data: {
                oldPath: file.path,
                newName: newName.trim()
            },
            dataType: 'json',
            success: function(response) {
                console.log('重命名成功响应:', response);
                showToast('重命名成功', 'success');
                
                // 强制刷新 - 先清空缓存数据
                console.log('清空缓存数据');
                fileList = [];
                directoryTree = {};
                
                // 重新加载文件列表
                console.log('重新加载文件列表，当前路径:', currentPath);
                loadFileList(currentPath);
                
                // 重新加载目录树
                console.log('重新加载目录树');
                loadDirectoryTree();
                
                // 额外的强制刷新（双重保险）
                setTimeout(() => {
                    console.log('额外的强制刷新');
                    loadFileList(currentPath);
                    loadDirectoryTree();
                }, 500);
            },
            error: function(xhr, status, error) {
                let errorMsg = '重命名失败';
                if (xhr.responseJSON && xhr.responseJSON.error) {
                    errorMsg += ': ' + xhr.responseJSON.error;
                } else {
                    errorMsg += ': ' + error;
                }
                showToast(errorMsg, 'error');
            }
        });
    }
}

// 删除文件
function deleteFile(file) {
    if (confirm(`确定要删除 ${file.name || '未知文件'} 吗？`)) {
        let apiUrl = apiBasePath + `api/file/delete/${file.path}`;
        console.log('删除文件API URL:', apiUrl);
        
        // 确保API基础路径正确
        if (!apiUrl.startsWith('http')) {
            if (!apiUrl.startsWith('/')) {
                apiUrl = '/' + apiUrl;
            }
        }
        
        console.log('最终API URL:', apiUrl);
        
        $.ajax({
            url: apiUrl,
            type: 'DELETE',
            dataType: 'json',
            success: function(response) {
                showToast('删除成功', 'success');
                // 重新加载文件列表
                loadFileList(currentPath);
                // 重新加载目录树
                loadDirectoryTree();
            },
            error: function(xhr, status, error) {
                console.error('删除文件失败:', error, xhr.responseText);
                console.error('HTTP状态码:', xhr.status);
                let errorMsg = '删除失败';
                if (xhr.responseJSON && xhr.responseJSON.error) {
                    errorMsg += ': ' + xhr.responseJSON.error;
                } else if (xhr.responseText) {
                    errorMsg += ': ' + xhr.responseText;
                } else {
                    errorMsg += ': ' + error;
                }
                showToast(errorMsg, 'error');
            }
        });
    }
}

// 格式化文件大小
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    
    let k = 1024;
    let sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// 显示提示消息
function showToast(message, type = 'info') {
    // 创建提示元素
    let toast = $('<div class="toast" role="alert" aria-live="assertive" aria-atomic="true"></div>');
    
    // 设置样式
    let bgColor = 'bg-info';
    switch (type) {
        case 'success':
            bgColor = 'bg-success';
            break;
        case 'error':
            bgColor = 'bg-danger';
            break;
        case 'warning':
            bgColor = 'bg-warning';
            break;
    }
    
    // 设置内容
    toast.html(`
        <div class="toast-header ${bgColor} text-white">
            <strong class="me-auto">提示</strong>
            <button type="button" class="btn-close btn-close-white" data-bs-dismiss="toast" aria-label="关闭"></button>
        </div>
        <div class="toast-body">
            ${message}
        </div>
    `);
    
    // 添加到容器
    $('#toast-container').append(toast);
    
    // 初始化并显示
    let toastInstance = new bootstrap.Toast(toast);
    toastInstance.show();
    
    // 自动隐藏
    setTimeout(() => {
        toastInstance.hide();
        setTimeout(() => {
            toast.remove();
        }, 300);
    }, 3000);
}