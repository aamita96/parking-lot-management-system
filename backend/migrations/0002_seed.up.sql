-- Default rate card (SRS 5.3, placeholder until confirmed), in paise.
INSERT INTO pricing_rules (vehicle_type, hourly_rate_paise, daily_cap_paise, grace_minutes, effective_from) VALUES
  ('MOTORCYCLE', 2000, 15000, 10, now()),
  ('CAR', 4000, 30000, 10, now()),
  ('TRUCK', 10000, 100000, 10, now());

INSERT INTO lot_settings (id, overflow_enabled) VALUES (true, false);

-- Simulator gate devices (used only when SIMULATOR_ENABLED=true).
INSERT INTO gate_devices (name, kind, is_simulator) VALUES
  ('SIM-IN', 'ENTRY', true),
  ('SIM-OUT', 'EXIT', true);

-- Reference layout from SRS section 11: rows A and B, six bays each.
INSERT INTO parking_spots (code, spot_type, floor, row_label, position) VALUES
  ('A1', 'COMPACT',   0, 'A', 1),
  ('A2', 'COMPACT',   0, 'A', 2),
  ('A3', 'REGULAR',   0, 'A', 3),
  ('A4', 'REGULAR',   0, 'A', 4),
  ('A5', 'REGULAR',   0, 'A', 5),
  ('A6', 'OVERSIZED', 0, 'A', 6),
  ('B1', 'COMPACT',   0, 'B', 1),
  ('B2', 'REGULAR',   0, 'B', 2),
  ('B3', 'REGULAR',   0, 'B', 3),
  ('B4', 'REGULAR',   0, 'B', 4),
  ('B5', 'OVERSIZED', 0, 'B', 5),
  ('B6', 'OVERSIZED', 0, 'B', 6);
