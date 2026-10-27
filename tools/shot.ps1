param(
  [string]$Out = "bin\\shot.png",
  [int]$Wait = 30,
  [switch]$Kill
)
# Captures the CODM Tune window as a PNG. The window is located by process
# name so the script does not depend on a hard-coded handle.
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class W {
  [DllImport("user32.dll")] public static extern IntPtr FindWindowW(string c, string n);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L, T, R, B; }
}
"@
# The HUD runs as a second codmtune.exe, so picking the first process with a
# window can capture the 320 px overlay instead of the main 1100 px window.
# Choose the widest window instead.
$best = $null; $bestW = 0
foreach ($q in (Get-Process codmtune -ErrorAction SilentlyContinue)) {
  if ($q.MainWindowHandle -eq 0) { continue }
  $rr = New-Object W+RECT
  [void][W]::GetWindowRect($q.MainWindowHandle, [ref]$rr)
  $ww = $rr.R - $rr.L
  if ($ww -gt $bestW) { $bestW = $ww; $best = $q }
}
if (-not $best) { Write-Error "codmtune window not found"; exit 1 }
$p = $best
$h = $p.MainWindowHandle
[void][W]::SetForegroundWindow($h)
Start-Sleep -Milliseconds $Wait
$r = New-Object W+RECT
[void][W]::GetWindowRect($h, [ref]$r)
$w = $r.R - $r.L; $ht = $r.B - $r.T
if ($w -le 0 -or $ht -le 0) { Write-Error "bad rect $w x $ht"; exit 1 }
$bmp = New-Object System.Drawing.Bitmap $w, $ht
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.L, $r.T, 0, 0, $bmp.Size)
$dir = Split-Path -Parent $Out
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir | Out-Null }
$bmp.Save((Resolve-Path -LiteralPath ".").Path + "\" + $Out, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()
Write-Output "$Out window=$w x $ht pid=$($p.Id)"
if ($Kill) { Stop-Process -Id $p.Id -Force }
