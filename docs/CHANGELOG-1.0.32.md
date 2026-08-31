# RemoteIt 1.0.32

- Перед созданием DXGI/GDI ресурсов экранный helper явно открывает session-local `winsta0` и назначает её процессу через `SetProcessWindowStation`.
- Исправлена ситуация Windows Server, когда `CreateProcessAsUser(..., "winsta0\\default")` оставлял helper в неинтерактивной service station и DXGI возвращал `E_ACCESSDENIED`.
- Закрепление за конкретным Windows SID и session ID, автоматический reconnect и точная VDI-диагностика сохранены.
