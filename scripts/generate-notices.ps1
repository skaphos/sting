# SPDX-License-Identifier: MIT

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$GoLicensesPackage = "github.com/google/go-licenses/v2@v2.0.1"
$OutputRoot = Join-Path $RepoRoot "third_party_licenses"
$ExceptionsFile = Join-Path $RepoRoot "scripts/license-exceptions.tsv"

$RuntimeOutputDir = Join-Path $OutputRoot "runtime"
$RuntimeReport = Join-Path $OutputRoot "runtime-report.csv"

New-Item -ItemType Directory -Force -Path $OutputRoot | Out-Null

foreach ($Path in @($RuntimeOutputDir)) {
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
}

# Modules go-licenses cannot classify; see scripts/license-exceptions.tsv.
$Exceptions = Get-Content -LiteralPath $ExceptionsFile |
    Where-Object { $_ -notmatch '^\s*#' -and $_.Trim() -ne "" } |
    ForEach-Object {
        $Fields = $_ -split "`t"
        [PSCustomObject]@{
            Module      = $Fields[0]
            Spdx        = $Fields[1]
            LicenseFile = $Fields[2]
        }
    }

# Replace the "Unknown,Unknown" rows go-licenses emits for an excepted module
# with a single canonical row carrying the manually verified SPDX identifier.
function Update-ReportRows {
    param(
        [string]$ReportPath,
        [string]$Module,
        [string]$Spdx,
        [string]$LicenseFile,
        [string]$Version
    )

    $Kept = Get-Content -LiteralPath $ReportPath |
        Where-Object { $_ -notmatch "^$([regex]::Escape($Module))(/|,)" }
    $Canonical = "{0},https://{0}/blob/{1}/{2},{3}" -f $Module, $Version, $LicenseFile, $Spdx
    $Rows = @($Kept) + $Canonical | Sort-Object -CaseSensitive
    Set-Content -LiteralPath $ReportPath -Value $Rows
}

# go-licenses skipped these modules, so copy their license files in ourselves.
function Save-ExceptionLicense {
    param(
        [string]$Module,
        [string]$LicenseFile,
        [string]$ModuleDir,
        [string]$SavePath
    )

    $Destination = Join-Path $SavePath $Module
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    $Target = Join-Path $Destination $LicenseFile
    Copy-Item -LiteralPath (Join-Path $ModuleDir $LicenseFile) -Destination $Target -Force
    Set-ItemProperty -LiteralPath $Target -Name IsReadOnly -Value $false
}

function Invoke-NoticeGeneration {
    param(
        [string]$ModuleDir,
        [string]$PackageArg,
        [string]$IgnorePrefix,
        [string]$ReportPath,
        [string]$SavePath
    )

    $IgnoreArgs = @("--ignore", $IgnorePrefix)
    foreach ($Exception in $Exceptions) {
        $IgnoreArgs += @("--ignore", $Exception.Module)
    }

    Push-Location $ModuleDir
    try {
        $report = go run $GoLicensesPackage report $PackageArg --ignore $IgnorePrefix
        Set-Content -LiteralPath $ReportPath -Value $report
        go run $GoLicensesPackage save $PackageArg @IgnoreArgs --save_path $SavePath

        foreach ($Exception in $Exceptions) {
            $DepDir = go list -m -f '{{.Dir}}' $Exception.Module
            $DepVersion = go list -m -f '{{.Version}}' $Exception.Module
            Update-ReportRows -ReportPath $ReportPath -Module $Exception.Module `
                -Spdx $Exception.Spdx -LicenseFile $Exception.LicenseFile -Version $DepVersion
            Save-ExceptionLicense -Module $Exception.Module -LicenseFile $Exception.LicenseFile `
                -ModuleDir $DepDir -SavePath $SavePath
        }
    }
    finally {
        Pop-Location
    }
}

Invoke-NoticeGeneration -ModuleDir $RepoRoot -PackageArg "./cmd/sting" -IgnorePrefix "github.com/skaphos/sting" -ReportPath $RuntimeReport -SavePath $RuntimeOutputDir

Write-Host ("Updated third-party notices in {0}" -f $OutputRoot)
