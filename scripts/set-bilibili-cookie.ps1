param([switch]$SelfTest)
$ErrorActionPreference = 'Stop'
function ConvertTo-BiliCookieConfig([string]$InputText) {
    $value = $InputText.Trim()
    if ($value.StartsWith('Cookie:', [StringComparison]::OrdinalIgnoreCase)) {
        $value = $value.Substring(7).Trim()
    }
    if ($value.Length -gt 32768 -or $value -match '[\r\n]' -or $value -notmatch '(?:^|;\s*)SESSDATA=[^;\s]+') {
        throw 'Cookie格式不完整：请复制登录后请求头中的完整Cookie值，须含SESSDATA且不换行。'
    }
    return (@{bilibili_cookie=$value} | ConvertTo-Json -Compress)
}
if ($SelfTest) {
    foreach ($value in @('SESSDATA=synthetic; bili_jct=fixture', 'Cookie: SESSDATA=synthetic; field="quoted"')) {
        $json=ConvertTo-BiliCookieConfig $value
        $decoded=$json|ConvertFrom-Json
        if($decoded.bilibili_cookie -notmatch '^SESSDATA=synthetic;'){throw 'Round-trip failed'}
    }
    foreach ($value in @('', 'bili_jct=fixture', "SESSDATA=synthetic`r`nInjected: value")) {
        $rejected=$false
        try{$null=ConvertTo-BiliCookieConfig $value}catch{$rejected=$true}
        if(-not $rejected){throw 'Invalid input accepted'}
    }
    Write-Output '5 synthetic checks passed; no configuration written.'
    return
}
$root=Split-Path -Parent $PSScriptRoot
$destination=Join-Path $root 'data/Bilibili/config.json'
$secret=Read-Host '粘贴B站请求头Cookie值并回车（输入隐藏）' -AsSecureString
$ptr=[IntPtr]::Zero
$temp=$null
try {
    $ptr=[Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
    $json=ConvertTo-BiliCookieConfig ([Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr))
    $directory=Split-Path -Parent $destination
    [IO.Directory]::CreateDirectory($directory)|Out-Null
    if(Test-Path -LiteralPath $destination){
        $backup=Join-Path $root '.local/bili-cookie-backups'
        [IO.Directory]::CreateDirectory($backup)|Out-Null
        Copy-Item -LiteralPath $destination -Destination (Join-Path $backup (([Guid]::NewGuid().ToString())+'.json'))
    }
    $temp=Join-Path $directory (([Guid]::NewGuid().ToString())+'.tmp')
    [IO.File]::WriteAllText($temp,$json,[Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temp -Destination $destination -Force
    Write-Output 'B站Cookie已保存到本机配置。请只反馈“已保存”；如替换了旧Cookie，需重启ZeroBot刷新各模块缓存。'
} finally {
    if($ptr -ne [IntPtr]::Zero){[Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)}
    $secret.Dispose()
    $json=$null
    if($temp -and (Test-Path -LiteralPath $temp)){Remove-Item -LiteralPath $temp -Force}
}
