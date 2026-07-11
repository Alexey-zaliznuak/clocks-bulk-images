import { Navigate, NavLink, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import Create from "./pages/Create";
import History from "./pages/History";
import type { ReactNode } from "react";

function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthed } = useAuth();
  return isAuthed ? <>{children}</> : <Navigate to="/login" replace />;
}

function Layout({ children }: { children: ReactNode }) {
  const { logout } = useAuth();
  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `px-4 py-2 rounded-full text-sm font-medium transition ${
      isActive ? "bg-blue-600 text-white shadow-sm" : "text-slate-600 hover:bg-slate-100"
    }`;

  return (
    <div className="min-h-full bg-slate-50 text-slate-800">
      <header className="border-b border-slate-200 bg-white/80 backdrop-blur sticky top-0 z-10">
        <div className="mx-auto max-w-6xl px-4 py-3 flex items-center gap-3">
          <div className="flex items-center gap-2 mr-4">
            <span className="grid place-items-center w-9 h-9 rounded-2xl bg-blue-600 text-white text-lg shadow-sm">
              ⏱️
            </span>
            <span className="font-bold tracking-tight text-slate-900">Named Clocks</span>
          </div>
          <nav className="flex gap-1">
            <NavLink to="/" end className={linkClass}>
              Создать
            </NavLink>
            <NavLink to="/history" className={linkClass}>
              История
            </NavLink>
          </nav>
          <button
            onClick={logout}
            className="ml-auto px-3 py-2 rounded-full text-sm text-slate-500 hover:text-slate-900 hover:bg-slate-100 transition"
          >
            Выйти
          </button>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">{children}</main>
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <Layout>
              <Create />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/history"
        element={
          <ProtectedRoute>
            <Layout>
              <History />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
