import { Navigate, Route, Routes } from "react-router-dom"
import AppShell from "./components/layout/AppShell"
import LoginPage from "./pages/LoginPage"
import RoomsPage from "./pages/RoomsPage"
import RoomPage from "./pages/RoomPage"
import LivePage from "./pages/LivePage"
import GuestJoinPage from "./pages/GuestJoinPage"
import RegisterPage from "./pages/RegisterPage"

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/registro" element={<RegisterPage />} />
        <Route path="/unirse" element={<GuestJoinPage />} />
        <Route path="/salas" element={<RoomsPage />} />
        <Route path="/salas/:id" element={<RoomPage />} />
        <Route path="/salas/:id/en-vivo" element={<LivePage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Route>
    </Routes>
  )
}
