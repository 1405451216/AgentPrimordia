Get-ChildItem internal/agent/dag/*.go | ForEach-Object {
    $bytes = [System.IO.File]::ReadAllBytes($_.FullName)
    # Check for UTF-8 BOM (EF BB BF) at the start
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $content = [System.Text.Encoding]::UTF8.GetString($bytes, 3, $bytes.Length - 3)
    } else {
        $content = [System.Text.Encoding]::UTF8.GetString($bytes)
    }
    # Remove any BOM characters in the middle
    $content = $content -replace [char]0xFEFF, ''
    # Write without BOM
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($_.FullName, $content, $utf8NoBom)
    Write-Host "Fixed BOM: $($_.Name)"
}
