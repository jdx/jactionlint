Write-Host $env:GITHUB_ENV
if ($true) { Get-ChildItem | % { $_.Name } }
"A=1" | Out-File -FilePath $env:GITHUB_ENV -Append
