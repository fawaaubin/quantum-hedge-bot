// Charger Chart.js
import Chart from "https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js";

// Connexion SocketIO
const socket = io("http://localhost:8080");

// Graphique des prix
const ctxPrice = document.getElementById("priceChart").getContext("2d");
const priceChart = new Chart(ctxPrice, {
  type: "line",
  data: {
    labels: [],
    datasets: [{
      label: "Prix",
      data: [],
      borderColor: "#0f0",
      backgroundColor: "rgba(0,255,0,0.1)",
      fill: true
    }]
  },
  options: { responsive: true }
});

// Graphique du capital
const ctxCapital = document.getElementById("capitalChart").getContext("2d");
const capitalChart = new Chart(ctxCapital, {
  type: "line",
  data: {
    labels: [],
    datasets: [{
      label: "Capital",
      data: [],
      borderColor: "#00f",
      backgroundColor: "rgba(0,0,255,0.1)",
      fill: true
    }]
  },
  options: { responsive: true }
});

// Graphique du drawdown
const ctxDD = document.getElementById("drawdownChart").getContext("2d");
const drawdownChart = new Chart(ctxDD, {
  type: "line",
  data: {
    labels: [],
    datasets: [{
      label: "Drawdown (%)",
      data: [],
      borderColor: "#f00",
      backgroundColor: "rgba(255,0,0,0.1)",
      fill: true
    }]
  },
  options: { responsive: true }
});

// Mise à jour temps réel via SocketIO
socket.on("price_update", data => {
  const now = new Date().toLocaleTimeString();

  // Prix
  priceChart.data.labels.push(now);
  priceChart.data.datasets[0].data.push(data.price);
  if (priceChart.data.labels.length > 50) {
    priceChart.data.labels.shift();
    priceChart.data.datasets[0].data.shift();
  }
  priceChart.update();

  // Signaux affichés en texte
  document.getElementById("signals").textContent = JSON.stringify(data.signals, null, 2);
});

// Mise à jour du capital et drawdown via API REST
async function updateStatus() {
  const res = await fetch("http://localhost:8080/status");
  const json = await res.json();
  const now = new Date().toLocaleTimeString();

  capitalChart.data.labels.push(now);
  capitalChart.data.datasets[0].data.push(json.capital);
  if (capitalChart.data.labels.length > 50) {
    capitalChart.data.labels.shift();
    capitalChart.data.datasets[0].data.shift();
  }
  capitalChart.update();

  drawdownChart.data.labels.push(now);
  drawdownChart.data.datasets[0].data.push(json.drawdown * 100);
  if (drawdownChart.data.labels.length > 50) {
    drawdownChart.data.labels.shift();
    drawdownChart.data.datasets[0].data.shift();
  }
  drawdownChart.update();
}
setInterval(updateStatus, 5000);
