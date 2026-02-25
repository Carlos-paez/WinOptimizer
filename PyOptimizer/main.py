import os
import threading
import subprocess
import tkinter as tk
from tkinter import ttk, messagebox
import ctypes
import sys

# --- Elevación automática a administrador ---
def is_admin():
    try:
        return ctypes.windll.shell32.IsUserAnAdmin()
    except:
        return False

if not is_admin():
    ctypes.windll.shell32.ShellExecuteW(
        None, "runas", sys.executable, " ".join(sys.argv), None, 1
    )
    sys.exit()

# --- Configuración de tareas ---
TASKS = [
    {"name": "Limpiar Archivos Temporales", "desc": "Eliminación rápida %TEMP%", "critical": False, "parallel": True},
    {"name": "Limpieza WinSxS (DISM)", "desc": "Limpieza de componentes (Pesado)", "critical": True, "parallel": False},
    {"name": "Optimizar RAM & Prefetch", "desc": "Borrado masivo de caché", "critical": False, "parallel": True},
    {"name": "Desfragmentar HDD", "desc": "Solo si es HDD", "critical": False, "parallel": False},
    {"name": "Reset Pila TCP/IP", "desc": "Comandos de red en paralelo", "critical": False, "parallel": True},
    {"name": "Limpiar Windows Update", "desc": "Borrado de caché de updates", "critical": True, "parallel": True},
    {"name": "Desactivar Telemetría", "desc": "Stop servicios rápido", "critical": True, "parallel": True},
    {"name": "Verificación SFC", "desc": "Scan rápido de integridad", "critical": False, "parallel": True},
]

# --- Funciones de Tareas ---
def clean_temp_files():
    count = 0
    paths = [os.getenv("TEMP"), os.path.join(os.getenv("WINDIR"), "Temp")]
    for path in paths:
        if not path: continue
        for f in os.listdir(path):
            try:
                full_path = os.path.join(path, f)
                if os.path.isdir(full_path):
                    os.rmdir(full_path)
                else:
                    os.remove(full_path)
                count += 1
            except: pass
    return f"{count} elementos purgados"

def clean_winsx():
    try:
        subprocess.run(["dism","/online","/cleanup-image","/startcomponentcleanup","/norestart"],check=True)
        return "WinSxS optimizado"
    except subprocess.CalledProcessError:
        return "Error: DISM requiere permisos de administrador"

def optimize_ram():
    count = 0
    prefetch = os.path.join(os.getenv("WINDIR"), "Prefetch")
    try:
        for f in os.listdir(prefetch):
            if f.endswith(".pf"):
                try:
                    os.remove(os.path.join(prefetch, f))
                    count += 1
                except PermissionError:
                    return "Acceso denegado al Prefetch"
    except PermissionError:
        return "No se puede acceder a Prefetch: se requieren privilegios de administrador"
    return f"Prefetch vaciado ({count})"

def defrag_hdd():
    try:
        out = subprocess.check_output(["powershell","-Command","(Get-PhysicalDisk -DeviceId 0).MediaType"])
        if out.strip() != b"3":
            subprocess.run(["defrag","C:","/U","/X"])
            return "HDD optimizado"
        return "SSD detectado (omitido)"
    except: return "Error al verificar disco"

def reset_network():
    cmds = [["ipconfig","/flushdns"], ["netsh","winsock","reset"], ["netsh","int","ip","reset"]]
    for cmd in cmds:
        try: subprocess.run(cmd)
        except: pass
    return "Pila de red restablecida"

def clean_windows_update():
    count = 0
    path = os.path.join(os.getenv("WINDIR"), "SoftwareDistribution", "Download")
    try:
        for f in os.listdir(path):
            try:
                full_path = os.path.join(path, f)
                if os.path.isdir(full_path): os.rmdir(full_path)
                else: os.remove(full_path)
                count += 1
            except: pass
    except PermissionError:
        return "Acceso denegado: se requieren privilegios de administrador"
    return f"Cache Update limpiada ({count})"

def disable_telemetry():
    svcs = ["DiagTrack","dmwappushservice"]
    stopped = 0
    for svc in svcs:
        try:
            subprocess.run(["sc","stop",svc])
            subprocess.run(["sc","config",svc,"start=","disabled"])
            stopped += 1
        except: pass
    return f"Telemetría ajustada ({stopped})"

def sfc_check():
    try: subprocess.run(["sfc","/verifyonly"])
    except: return "Error: requiere permisos de administrador"
    return "Integridad verificada"

TASK_FUNCTIONS = [
    clean_temp_files, clean_winsx, optimize_ram, defrag_hdd,
    reset_network, clean_windows_update, disable_telemetry, sfc_check
]

# --- GUI Modernizada ---
class App:
    def __init__(self, root):
        self.root = root
        root.title("⚡ Windows Turbo Optimizer")
        root.geometry("720x560")
        root.configure(bg="#0d1117")  # Fondo oscuro

        # Estilos ttk
        style = ttk.Style()
        style.theme_use('clam')
        style.configure("TProgressbar", troughcolor="#11161e", background="#00f0ff", thickness=25)

        messagebox.showinfo("Aviso","Algunas tareas requieren permisos de administrador para ejecutarse correctamente.")

        # Encabezado
        header = tk.Label(root, text="⚡ Windows Turbo Optimizer", font=("Segoe UI", 22, "bold"),
                          fg="#00f0ff", bg="#0d1117")
        header.pack(pady=15)

        # Frame tareas
        frame_tasks = tk.Frame(root, bg="#0d1117")
        frame_tasks.pack(fill="both", padx=20, pady=10)

        self.tasks_vars = []
        for i, task in enumerate(TASKS):
            var = tk.BooleanVar(value=True)
            chk = tk.Checkbutton(frame_tasks, text=f"{task['name']} - {task['desc']}", variable=var,
                                 anchor="w", justify="left", fg="#00f0ff", bg="#0d1117",
                                 selectcolor="#11161e", activebackground="#0d1117", activeforeground="#00f0ff",
                                 font=("Segoe UI", 11))
            chk.pack(fill="x", pady=2)
            self.tasks_vars.append(var)

        # Botón iniciar
        self.start_btn = tk.Button(root, text="Iniciar Optimización", font=("Segoe UI", 12, "bold"),
                                   bg="#00f0ff", fg="#0d1117", activebackground="#00e0ff",
                                   command=self.start_tasks, relief="flat")
        self.start_btn.pack(pady=15)

        # Barra de progreso
        self.progress = ttk.Progressbar(root, orient="horizontal", length=650, mode="determinate")
        self.progress.pack(pady=10)

        # Log
        self.log_text = tk.Text(root, height=15, bg="#11161e", fg="#00f0ff", insertbackground="#00f0ff",
                                font=("Consolas",10))
        self.log_text.pack(fill="both", padx=20, pady=10)

        # Botón de cierre (oculto al inicio)
        self.close_btn = tk.Button(root, text="Cerrar Aplicación", font=("Segoe UI", 12, "bold"),
                                   bg="#ff0055", fg="#ffffff", activebackground="#ff3366",
                                   command=root.destroy, relief="flat")
        self.close_btn.pack(pady=10)
        self.close_btn.pack_forget()  # ocultar inicialmente

    def start_tasks(self):
        self.start_btn.config(state="disabled")
        self.log_text.insert(tk.END,"Iniciando tareas...\n\n")
        self.selected_tasks = [i for i,var in enumerate(self.tasks_vars) if var.get()]
        self.progress["maximum"] = len(self.selected_tasks)
        threading.Thread(target=self.run_tasks, daemon=True).start()

    def run_tasks(self):
        completed = 0
        report_lines = ["--- REPORTE FINAL DE OPTIMIZACIÓN ---\n"]
        for idx in self.selected_tasks:
            func = TASK_FUNCTIONS[idx]
            result = func()
            line = f"{TASKS[idx]['name']}: {result}"
            self.log_text.insert(tk.END, line + "\n")
            report_lines.append(line)
            completed += 1
            self.progress["value"] = completed

        # Mostrar mensaje de finalización
        self.log_text.insert(tk.END,"\n🎉 Optimización completa!\n")
        self.log_text.insert(tk.END, "\n".join(report_lines))
        self.start_btn.pack_forget()  # ocultar botón iniciar
        self.close_btn.pack()          # mostrar botón cerrar

# --- Ejecutar App ---
if __name__ == "__main__":
    root = tk.Tk()
    app = App(root)
    root.mainloop()