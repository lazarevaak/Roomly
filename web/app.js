const loginSection = document.querySelector("#login-section");
const appSection = document.querySelector("#app-section");
const userPanel = document.querySelector("#user-panel");
const currentUser = document.querySelector("#current-user");

const loginForm = document.querySelector("#login-form");
const usernameInput = document.querySelector("#username");
const passwordInput = document.querySelector("#password");
const registerForm = document.querySelector("#register-form");
const registerUsernameInput = document.querySelector("#register-username");
const registerPasswordInput = document.querySelector("#register-password");
const passwordForm = document.querySelector("#password-form");
const currentPasswordInput = document.querySelector("#current-password");
const newPasswordInput = document.querySelector("#new-password");
const logoutButton = document.querySelector("#logout-button");

const roomAdminPanel = document.querySelector("#room-admin-panel");
const roomForm = document.querySelector("#room-form");
const roomNameInput = document.querySelector("#room-name");
const roomCapacityInput = document.querySelector("#room-capacity");
const roomLocationInput = document.querySelector("#room-location");
const roomFormTitle = document.querySelector("#room-form-title");
const roomSaveButton = document.querySelector("#room-save-button");
const roomCancelButton = document.querySelector("#room-cancel-button");

const bookingForm = document.querySelector("#booking-form");
const bookingRoomSelect = document.querySelector("#booking-room");
const bookingPurposeInput = document.querySelector("#booking-purpose");
const bookingStartInput = document.querySelector("#booking-start");
const bookingEndInput = document.querySelector("#booking-end");
const bookingFormTitle = document.querySelector("#booking-form-title");
const bookingSaveButton = document.querySelector("#booking-save-button");
const bookingCancelButton = document.querySelector("#booking-cancel-button");

const refreshButton = document.querySelector("#refresh-button");
const message = document.querySelector("#message");
const roomsContainer = document.querySelector("#rooms");
const bookingsContainer = document.querySelector("#bookings");

const state = {
  rooms: [],
  bookings: [],
  user: null,
  editingRoomId: null,
  editingBookingId: null,
  busy: false
};

const apiClient = {
  async request(path, options = {}) {
    const response = await fetch(path, options);

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text.trim() || `Ошибка HTTP ${response.status}`);
    }

    if (response.status === 204) {
      return null;
    }

    return response.json();
  },

  me() {
    return this.request("/api/me");
  },

  login(username, password) {
    return this.request("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password })
    });
  },

  register(username, password) {
    return this.request("/api/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password })
    });
  },

  logout() {
    return this.request("/api/logout", { method: "POST" });
  },

  changePassword(currentPassword, newPassword) {
    return this.request("/api/password", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ currentPassword, newPassword })
    });
  },

  rooms() {
    return this.request("/api/rooms");
  },

  saveRoom(room, id) {
    return this.request(id === null ? "/api/rooms" : `/api/rooms/${id}`, {
      method: id === null ? "POST" : "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(room)
    });
  },

  deleteRoom(id) {
    return this.request(`/api/rooms/${id}`, { method: "DELETE" });
  },

  bookings() {
    return this.request("/api/bookings");
  },

  saveBooking(booking, id) {
    return this.request(id === null ? "/api/bookings" : `/api/bookings/${id}`, {
      method: id === null ? "POST" : "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(booking)
    });
  },

  deleteBooking(id) {
    return this.request(`/api/bookings/${id}`, { method: "DELETE" });
  }
};

function isAdmin() {
  return state.user?.role === "admin";
}

function roleName(role) {
  return role === "admin" ? "администратор" : "пользователь";
}

function setBusy(value) {
  state.busy = value;
  document.querySelectorAll("button, input, select").forEach((element) => {
    element.disabled = value;
  });
}

function showApp(user) {
  state.user = user;
  loginSection.hidden = true;
  appSection.hidden = false;
  userPanel.hidden = false;
  currentUser.textContent = `Пользователь: ${user.username} · ${roleName(user.role)}`;
  roomAdminPanel.hidden = !isAdmin();
}

function showLogin() {
  state.user = null;
  loginSection.hidden = false;
  appSection.hidden = true;
  userPanel.hidden = true;
  usernameInput.value = "";
  passwordInput.value = "";
}

function resetRoomForm() {
  state.editingRoomId = null;
  roomForm.reset();
  roomFormTitle.textContent = "Добавить комнату";
  roomSaveButton.textContent = "Добавить";
  roomCancelButton.hidden = true;
}

function resetBookingForm() {
  state.editingBookingId = null;
  bookingForm.reset();
  bookingFormTitle.textContent = "Добавить бронирование";
  bookingSaveButton.textContent = "Забронировать";
  bookingCancelButton.hidden = true;
}

function renderRooms() {
  roomsContainer.replaceChildren();
  bookingRoomSelect.replaceChildren();

  for (const room of state.rooms) {
    const option = document.createElement("option");
    option.value = room.id;
    option.textContent = `${room.name} (${room.capacity} чел.)`;
    bookingRoomSelect.append(option);
  }

  if (state.rooms.length === 0) {
    roomsContainer.textContent = "Комнат пока нет.";
    return;
  }

  for (const room of state.rooms) {
    const card = document.createElement("article");
    card.className = "item";

    const title = document.createElement("h3");
    title.textContent = room.name;

    const details = document.createElement("p");
    details.textContent = `Вместимость: ${room.capacity} чел. · ${room.location}`;

    card.append(title, details);

    if (isAdmin()) {
      const editButton = document.createElement("button");
      editButton.type = "button";
      editButton.textContent = "Редактировать";
      editButton.addEventListener("click", () => startEditingRoom(room));

      const deleteButton = document.createElement("button");
      deleteButton.type = "button";
      deleteButton.textContent = "Удалить";
      deleteButton.className = "delete";
      deleteButton.addEventListener("click", () => deleteRoom(room));

      card.append(editButton, deleteButton);
    }

    roomsContainer.append(card);
  }
}

function renderBookings() {
  bookingsContainer.replaceChildren();

  if (state.bookings.length === 0) {
    bookingsContainer.textContent = "Бронирований пока нет.";
    return;
  }

  for (const booking of state.bookings) {
    const card = document.createElement("article");
    card.className = "item";

    const title = document.createElement("h3");
    title.textContent = booking.purpose;

    const details = document.createElement("p");
    details.textContent =
      `${booking.roomName} · ${formatDate(booking.startsAt)} — ${formatDate(booking.endsAt)}`;

    const owner = document.createElement("p");
    owner.textContent = `Создал: ${booking.username}`;

    const editButton = document.createElement("button");
    editButton.type = "button";
    editButton.textContent = "Редактировать";
    editButton.addEventListener("click", () => startEditingBooking(booking));

    const deleteButton = document.createElement("button");
    deleteButton.type = "button";
    deleteButton.textContent = "Удалить";
    deleteButton.className = "delete";
    deleteButton.addEventListener("click", () => deleteBooking(booking));

    card.append(title, details, owner, editButton, deleteButton);
    bookingsContainer.append(card);
  }
}

async function refreshData() {
  state.rooms = await apiClient.rooms();
  state.bookings = await apiClient.bookings();
  renderRooms();
  renderBookings();
}

function startEditingRoom(room) {
  if (state.busy) return;

  state.editingRoomId = room.id;
  roomNameInput.value = room.name;
  roomCapacityInput.value = room.capacity;
  roomLocationInput.value = room.location;
  roomFormTitle.textContent = "Редактировать комнату";
  roomSaveButton.textContent = "Сохранить";
  roomCancelButton.hidden = false;
  roomNameInput.focus();
}

function startEditingBooking(booking) {
  if (state.busy) return;

  state.editingBookingId = booking.id;
  bookingRoomSelect.value = booking.roomId;
  bookingPurposeInput.value = booking.purpose;
  bookingStartInput.value = toLocalInputValue(booking.startsAt);
  bookingEndInput.value = toLocalInputValue(booking.endsAt);
  bookingFormTitle.textContent = "Редактировать бронирование";
  bookingSaveButton.textContent = "Сохранить";
  bookingCancelButton.hidden = false;
  bookingPurposeInput.focus();
}

async function deleteRoom(room) {
  if (state.busy || !confirm(`Удалить комнату «${room.name}» и её бронирования?`)) {
    return;
  }

  await runAction(async () => {
    await apiClient.deleteRoom(room.id);
    message.textContent = "Комната удалена.";
    await refreshData();
  });
}

async function deleteBooking(booking) {
  if (state.busy || !confirm(`Удалить бронирование «${booking.purpose}»?`)) {
    return;
  }

  await runAction(async () => {
    await apiClient.deleteBooking(booking.id);
    message.textContent = "Бронирование удалено.";
    await refreshData();
  });
}

async function runAction(action) {
  setBusy(true);
  message.textContent = "";

  try {
    await action();
  } catch (error) {
    message.textContent = error.message;
  } finally {
    setBusy(false);
  }
}

function formatDate(value) {
  return new Date(value).toLocaleString("ru-RU", {
    dateStyle: "short",
    timeStyle: "short"
  });
}

function toLocalInputValue(value) {
  const date = new Date(value);
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function toAPIInstant(value) {
  return new Date(value).toISOString();
}

loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  await runAction(async () => {
    const user = await apiClient.login(
      usernameInput.value.trim(),
      passwordInput.value
    );

    showApp(user);
    await refreshData();
    message.textContent = "";
  });
});

registerForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  await runAction(async () => {
    const user = await apiClient.register(
      registerUsernameInput.value.trim(),
      registerPasswordInput.value
    );

    registerForm.reset();
    showApp(user);
    await refreshData();
    message.textContent = "";
  });
});

passwordForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  await runAction(async () => {
    await apiClient.changePassword(
      currentPasswordInput.value,
      newPasswordInput.value
    );

    passwordForm.reset();
    message.textContent = "Пароль изменён.";
  });
});

logoutButton.addEventListener("click", async () => {
  await runAction(async () => {
    await apiClient.logout();
    showLogin();
  });
});

roomForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const room = {
    name: roomNameInput.value.trim(),
    capacity: Number(roomCapacityInput.value),
    location: roomLocationInput.value.trim()
  };

  await runAction(async () => {
    await apiClient.saveRoom(room, state.editingRoomId);
    resetRoomForm();
    message.textContent = "Комната сохранена.";
    await refreshData();
  });
});

bookingForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const booking = {
    roomId: Number(bookingRoomSelect.value),
    purpose: bookingPurposeInput.value.trim(),
    startsAt: toAPIInstant(bookingStartInput.value),
    endsAt: toAPIInstant(bookingEndInput.value)
  };

  await runAction(async () => {
    await apiClient.saveBooking(booking, state.editingBookingId);
    resetBookingForm();
    message.textContent = "Бронирование сохранено.";
    await refreshData();
  });
});

roomCancelButton.addEventListener("click", resetRoomForm);
bookingCancelButton.addEventListener("click", resetBookingForm);

refreshButton.addEventListener("click", async () => {
  if (state.busy) return;
  await runAction(refreshData);
});

(async function boot() {
  try {
    const user = await apiClient.me();
    showApp(user);
    await refreshData();
  } catch {
    showLogin();
    setTimeout(showLogin, 100);
  }
})();
