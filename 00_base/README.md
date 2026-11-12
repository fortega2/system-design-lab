# Fase 0 · Base del laboratorio

Una sola instancia de la app en Go delante de un Postgres, con el stack de
observabilidad compartido y k6 como generador de carga. Es la línea base contra
la que se comparan todas las fases siguientes, y el primer lugar donde se rompe
algo a propósito.

![Arquitectura de la fase 0](diagrams/architecture.svg)

Líneas sólidas: el camino de un request. Punteadas: observabilidad. Fuente en
[`diagrams/architecture.d2`](diagrams/architecture.d2) (`make diagrams`).

## Cómo correrla

```bash
make up                               # observabilidad + Postgres + app
make load RATE=200 DURATION=5m        # k6 a tasa constante (ver load/constant.js)
make down                             # baja todo y conserva los volúmenes
```

Los recursos se cambian sin editar el compose: `make up PG_CPUS=2 APP_CPUS=1
DB_MAX_CONNS=20` y `make load RATE=35000 MAX_VUS=2000`. Sin variables quedan
los valores de la tabla de abajo. `make load` corre contra el stack que ya está
levantado y no lo toca (`--no-deps`): primero `make up`.

| Qué | Dónde |
|---|---|
| Dashboard RED | http://localhost:3000/d/lab-overview |
| App | http://localhost:8080 (`/healthz`, `/readyz`, `/items`, `/slow`) |
| Prometheus | http://localhost:9090 |

El script de k6 usa un modelo abierto (`constant-arrival-rate`): llegan `RATE`
requests por segundo pase lo que pase, 90% `GET /items/{id}` y 10%
`POST /items`. Si el sistema se pone lento, los requests se acumulan en vez de
bajar la carga sin que nadie lo note.

Para empezar con la base vacía: `make down && docker volume rm lab-00-base_postgres-data`.
Hace falta una vez para que exista el usuario `exporter`: los scripts de inicio
(`observability/postgres/init.sh`, que aplica `app/schema` y el rol de
monitoreo) solo corren cuando se crea el volumen.

### Configuración medida

| Componente | Límites y configuración |
|---|---|
| Postgres 18 | 1 CPU, 512 MB, `shared_buffers=64MB`, `max_connections=50` |
| postgres-exporter | 0,25 CPU, 64 MB; usuario `exporter` con `pg_monitor`, ocupa 1 de las 50 conexiones |
| app | 2 CPU, 256 MB, pool de 10 conexiones, **sin timeout de base** (`DB_TIMEOUT=0s`). Go 1.27 ajusta `GOMAXPROCS` al límite de CPU del contenedor, redondeado para arriba y con mínimo 2 (no hay que setearlo; setearlo apaga el ajuste) |
| k6 | hasta 500 VUs (`MAX_VUS`); timeout por request de 60 s (el default) |
| Host | Ryzen 5 5600X (6 núcleos / 12 hilos), 32 GB, NVMe; k6 corre en la misma máquina |

Los números de abajo valen para este entorno. Lo que se puede llevar a otro
lado son las formas de las curvas y los mecanismos, no los valores absolutos.

## E1 · Capacidad: ¿qué se satura primero?

**Hipótesis.** Postgres, limitado a 1 CPU, es lo primero que se satura. Con
carga moderada no debería pasar nada.

**Carga.** `make load` subiendo `RATE` de 200 a 28.000 req/s.

**Resultado.**

| RATE pedido | Atendido | p95 (k6) | CPU Postgres | CPU app | `dropped_iterations` |
|---|---|---|---|---|---|
| 200 | 200 | 0,8 ms | ~0 | ~0 | 0 |
| 3.000 | 3.000 | 0,9 ms | 0,15 | 0,2 | 13 |
| 10.000 | 9.960 | 1,2 ms | 0,62 | 0,48 | 1.154 |
| 15.000 | 14.910 | 6,8 ms | 0,88 | 1,0 | 9.467 |
| 20.000 | 15.600 | 47 ms | 0,92 | 0,92 | 790.856 |
| 28.000 | 15.570 | 62 ms | **1,00** | 0,90 | 1.489.803 |

- **El techo es ~15,6k req/s y lo pone la CPU de Postgres.** A partir de 20k
  está al 100% y el kernel lo frena (`container_cpu_cfs_throttled_seconds_total`
  sube a ~1,5 s/s). La app nunca pasa de 1 core de sus 2.
- **La cola se arma en el pool de la app.** A 20k, el 94% de los pedidos de
  conexión encontró el pool lleno y esperó, 21 ms en promedio. Casi toda la
  latencia es esa espera, no las queries.
- **Pasado el techo, más carga solo agrega latencia.** De 20k a 28k el
  throughput no se mueve y el p50 del server pasa de 17 a 21 ms.
- **Los VUs de k6 cumplen la ley de Little** (concurrencia = throughput ×
  latencia): 15,6k req/s × 31 ms ≈ 480 VUs, contra un tope de 500. Cuando se
  acaban, k6 descarta iteraciones (`dropped_iterations`).
- **El host no fue el límite:** todos los contenedores juntos usaron ~3,8 de
  12 hilos.

**En qué me equivoqué.** Esperaba ver la saturación a "carga moderada". Leer
y escribir por primary key es tan barato que, con 1 CPU, Postgres aguanta
~15k req/s, y a 3k está al 15%.

**Lo que no esperaba.**

- **Cero errores con el sistema saturado.** Sin timeouts nada falla: todo se
  vuelve más lento. Un cliente real con timeout se iría, y eso recién se vería
  como 499.
- **Los thresholds de k6 quedaron en ✓** (`p(95)<100ms` con p95 = 62 ms). La
  señal real fue `dropped_iterations`.

## E2 · Tamaño del pool de conexiones

**Hipótesis.** La regla clásica (wiki de PostgreSQL, HikariCP) es
`conexiones = cores × 2 + discos efectivos`, contando los cores **de Postgres**.
Con 1 CPU y los datos en memoria da ~3. Como el techo es la CPU de Postgres, el
throughput no debería cambiar con el tamaño del pool.

**Carga.** `RATE=20000` (por encima del techo, con k6 al tope de 500 VUs en
todas las corridas), cambiando `DB_MAX_CONNS` en `compose.yaml`.

**Resultado.**

| Pool | Atendido | CPU Postgres | Throttling | Esperaron conexión | Espera media | p50 / p99 | Memoria Postgres |
|---|---|---|---|---|---|---|---|
| 3 | 13,0k | 0,86 | 0,01 s/s | 100% | 38 ms | 38 / 82 ms | 37 MiB |
| 5 | 14,5k | 0,99 | 0,40 s/s | 100% | 33 ms | 36 / 88 ms | 42 MiB |
| 10 (día 1) | 15,6k | 0,92 | 1,54 s/s | 94% | 21 ms | 17 / 78 ms | 72 MiB |
| 10 (día 2) | 13,9k | 0,91 | 0,98 s/s | 100% | 32 ms | 28 / 98 ms | 66 MiB |
| 50 | 14,9k | 1,00 | 4,29 s/s | 73% | 22 ms | 9,5 / 91 ms | 141 MiB |

- **Con 3 conexiones, el pool pasa a ser el cuello de botella:** Postgres queda
  con CPU libre (0,86, sin frenar). Cada query pasa parte de su tiempo afuera de
  Postgres (ida y vuelta por la red de Docker, trabajo de la app), y con 3
  conexiones esos huecos no se llegan a cubrir.
- **Entre 5 y 50 la diferencia cae dentro del ruido.** La misma configuración
  (pool 10) dio 15,6k un día y 13,9k otro.
- **Pool 50 tiene costos medibles:** el doble de memoria (cada conexión es un
  proceso de Postgres), casi el triple de throttling (50 procesos peleando por
  1 CPU) y el peor p99. La mediana mejora porque uno de cada cuatro requests
  consigue conexión al instante, pero el throughput no sube.
- **Pool 50 = `max_connections` 50: la app se quedó con todas.** Después de la
  corrida ni `psql` podía entrar (`FATAL: sorry, too many clients already`). El
  pool mantiene abiertas las conexiones ociosas.

**En qué me equivoqué.** Predije que con 3 el throughput no iba a cambiar, y
cambió (-17%). Después afirmé que el óptimo era 10 con una sola corrida por
punto, y repetirla mostró que la diferencia entre 5, 10 y 50 no se sostiene.

**Conclusión.** La fórmula es un punto de partida que acá se queda corto, porque
no cuenta el tiempo que la conexión pasa fuera de la base. Para elegir el
tamaño hay que medir, y lo importante pasa en los extremos: con muy pocas
conexiones la base queda ociosa, y con muchas cuestan memoria y latencia de
cola y pueden agotar `max_connections`.

## E3 · Caída de Postgres sin timeout de base

**Hipótesis.** Al tirar Postgres, los requests no fallan: se cuelgan. Los
in-flight y las goroutines crecen, `/readyz` da 503 y, al volver Postgres, todo
se recupera solo. Si la caída dura más de 60 s, k6 corta los requests y la app
los registra como 499.

**Carga y falla.** `make load RATE=200 DURATION=3m`. Al minuto,
`docker stop lab-00-base-postgres-1`. 70 s después, `docker start`.

**Resultado.**

| Momento | In-flight | Goroutines | Respuestas OK/s | Qué pasa |
|---|---|---|---|---|
| Antes | 1 | 61 | 200 | normal |
| Corte (+0 s) | — | — | — | 11 errores 500: las queries en curso y las conexiones nuevas reciben `the database system is shutting down` |
| +5 s | 249 | 533 | 109 | los requests empiezan a colgarse |
| +10 s → +65 s | **~500** | **~1.040** | **0** | k6 llegó a su tope de 500 VUs; `/readyz` da 503 durante toda la caída |
| +60 s | — | — | — | k6 corta por timeout los requests colgados más viejos: **458 timeouts** |
| +70 s (vuelve Postgres) | 1 | 516 | 200 en < 5 s | recuperación sola, sin intervención |

Totales: k6 registró **518 fallas** (2,3%): **458 timeouts** de k6, que la app
contó exactamente como **458 respuestas 499**, y **60 errores 500**: 11 en el
instante del corte y 49 durante la caída (~1 por segundo), con
`no such host` porque el DNS de Docker ya no resuelve `postgres`. Además, k6 descartó 13.228
iteraciones por no tener VUs libres.

```mermaid
sequenceDiagram
    participant K as k6
    participant A as app
    participant P as pool (10)
    participant DB as Postgres
    Note over DB: docker stop
    K->>A: GET /items/42
    A->>P: Acquire()
    Note over P: sin conexiones utilizables,<br/>sin timeout de base
    P--xP: espera sin límite
    Note over K,A: el request queda colgado<br/>y ocupa un VU de k6
    K--xA: timeout de k6 a los 60 s
    Note over A: contexto cancelado → 499
    Note over DB: docker start
    K->>A: GET /items/17
    A->>P: Acquire()
    P->>DB: nueva conexión
    DB-->>A: fila
    A-->>K: 200
```

**Lo que se confirmó.** Sin timeout de base, una dependencia caída no produce
errores rápidos: produce requests colgados que se acumulan hasta el límite del
cliente. Y la falla se ve del lado del cliente (timeouts) antes que en las
métricas de error del servidor: durante 60 s el panel de 5xx está casi vacío.

**Lo que no esperaba.**

- **Las goroutines bajaron a la mitad y no a cero después de la recuperación**
  (516 durante casi un minuto). Son las conexiones keep-alive de los 500 VUs
  de k6: `net/http` mantiene una goroutine por conexión abierta. Bajaron a 10
  cuando k6 terminó.
- **Durante la caída, `/readyz` tardaba 2 s en contestar el 503**: espera su
  propio timeout contra la base. Un load balancer con un health check corto
  (por ejemplo 1 s) lo vería como timeout, no como 503.

**Pregunta abierta.** Durante la caída el pool reportaba 10 conexiones totales
y 0 en uso, mientras ~500 requests esperaban. No medí qué estaban haciendo
exactamente esas 10 (reintentos de conexión que no fallan rápido, por ejemplo).
Vale la pena investigarlo cuando se agregue el timeout de base.

## E4 · Sacar y poner recursos: ¿adónde se mueve el cuello de botella?

**Hipótesis.** El techo de E1 lo pone la CPU de Postgres, así que debería
seguir a esa CPU. Pero no indefinidamente: con más CPU en la base, el límite
pasa a otro lado.

| # | Postgres | app | Pool | Predicción |
|---|---|---|---|---|
| A | **0,5** | 2 | 10 | el techo cae casi a la mitad, ~7–8k |
| B | 1 | 2 | 10 | control: ~14–16k, como E1 |
| C | **2** | 2 | 10 | 22–27k, no ~31k: el límite pasa a ser la app o el host |
| D | 2 | 2 | **5** | el pool vuelve a ser el cuello: Postgres queda por debajo de 2 CPU |
| E | 2 | 2 | **20** | el mismo techo que C, con más throttling y peor p99 |
| F | 2 | **1** | 10 | la app pasa a ser el cuello (~15k) y Postgres queda ocioso |

**Carga.** Todas las corridas con `make load RATE=35000 DURATION=1m
MAX_VUS=2000`. Esa carga está por encima de cualquier techo, así que lo
atendido *es* el techo de cada configuración. Hice una sola corrida por
configuración, con las métricas de los últimos 45 s. Con el ±7% de ruido entre
corridas, las diferencias chicas (C contra E) son orientativas.

**Resultado.**

| # | Postgres / app / pool | Atendido | CPU Postgres | CPU app | Throttling | CPU k6 | CPU del lab | Conexión retenida | p50 / p99 server |
|---|---|---|---|---|---|---|---|---|---|
| A | 0,5 / 2 / 10 | 7,6k | **0,50** | 0,44 | Postgres 2,7 s/s | 1,0 | 2,0 | — | 300 / 496 ms |
| B | 1 / 2 / 10 | 15,2k | **1,00** | 0,78 | Postgres 1,5 s/s | 2,0 | 3,9 | — | 174 / 248 ms |
| C | 2 / 2 / 10 | 24,8k | 1,62 | 1,24 | ~0 | 3,3 | 6,2 | 400 µs | 77 / 230 ms |
| D | 2 / 2 / 5 | 18,2k | 1,19 | 1,18 | ~0 | 2,5 | 4,9 | 275 µs | 163 / 248 ms |
| E | 2 / 2 / 20 | 27,2k | 1,79 | 1,49 | ~0 | 3,6 | 6,9 | 735 µs | 76 / 200 ms |
| F | 2 / 1 / 10 | 17,4k | 1,13 | **1,00** | app 0,75 s/s | 2,3 | 4,5 | 575 µs | 172 / 248 ms |

En todas las corridas el 100% de los pedidos de conexión encontró el pool
lleno, y no hubo ningún error. "Conexión retenida" es pool ÷ throughput: cuánto
tiempo ocupa cada request una conexión. En A y B no se compara, porque ahí
la conexión espera adentro de un Postgres saturado. "CPU del lab" suma todos
los contenedores, k6 incluido, sobre 12 hilos.

- **El costo por request es la constante del sistema.** Postgres gasta ~66 µs
  de CPU por request en todas las corridas (0,50 / 7,6k = 1,79 / 27,2k). Por
  eso, mientras el límite es la CPU de la base, el techo es lineal: con la
  mitad de CPU se atiende exactamente la mitad (A contra B).
- **Duplicar la CPU de la base dio 1,63 veces más, no 2 (C).** Postgres quedó
  con 0,38 CPU libre y sin throttling, y la app tampoco estaba al tope. El
  límite pasó al pool. Visto con la ley de Little, el throughput es conexiones
  ÷ tiempo retenido: 10 ÷ 400 µs ≈ 25k.
- **El tiempo retenido crece con la carga del host.** Con 1 CPU y pool 3 (E2)
  era ~230 µs. Acá va de 275 µs (D, host más tranquilo) a 735 µs (E, 6,9 hilos
  ocupados en 6 núcleos físicos). Por eso agrandar el pool rinde cada vez menos:
  de 3,6k req/s por conexión con 5 a 1,4k con 20.
- **Con pool 20 no apareció el costo de E2 (E).** Fue +10% de throughput, mejor
  p99 y casi sin throttling. Lo que pesaba en E2 eran 50 conexiones para 1 CPU.
  Acá son 10 por CPU. El "demasiadas conexiones" depende de cuántas haya por
  core, no del número absoluto.
- **Con la app limitada, el síntoma aparece en el pool (F).** La app está
  clavada en 1,0 CPU y frenada 0,75 s/s, y Postgres tiene casi una CPU libre.
  Aun así, el dashboard del pool se ve igual que en D (100% de esperas): cuando
  el kernel congela la app, los requests retienen conexiones sin usarlas (575
  µs contra 400 µs). Mirando solo el pool uno lo agrandaría. La causa se ve en
  el throttling de cada contenedor.
- **Con saturación, la latencia mide la cola, no el trabajo.** En todas las
  corridas, throughput × latencia ≈ 2.000, el tope de VUs de k6. Con más VUs,
  la latencia sube y el throughput no cambia.

**En qué me equivoqué.**

- **C:** acerté el número (24,8k dentro de 22–27k) pero no la causa. Esperaba
  que limitara la app o el host, y limitó el pool. Mi cálculo de "con 8–10
  conexiones alcanza para 2 CPU" usaba el tiempo retenido de E2 como si fuera
  fijo, y crece con la carga.
- **E:** predije lo contrario de lo que pasó, porque extrapolé el número
  absoluto de E2 (pool 50) sin dividir por los cores.

**Lo que no esperaba.**

- **k6 es lo que más CPU consume:** 3,6 CPU en E, más que Postgres y la app
  juntos. En un host compartido, el generador de carga le roba CPU al sistema
  que mide.
- **`GOMAXPROCS` no baja de 2.** Con `APP_CPUS=1` quedó en 2. Dos hilos de Go
  gastan la cuota de 1 CPU en medio período y el resto del período quedan
  congelados: eso es el throttling de la app en F.
- **La app gastó casi la misma CPU en D que en C** (1,18 contra 1,24) con un
  27% menos de requests. Puede ser el costo de manejar 2.000 goroutines
  esperando 5 conexiones. No lo medí.

## E5 · Postgres por dentro

Con `postgres_exporter` se ve qué hacen las conexiones adentro de Postgres
(fila "Postgres" del dashboard). Repetí B y E de E4 mirando
`pg_stat_activity`.

**Hipótesis.** En B (1 CPU, pool 10), las 10 conexiones de la app están
activas casi todo el tiempo y mayormente en CPU. En E (2 CPU, pool 20) hay
más conexiones ociosas, porque el tiempo retenido está casi todo fuera de
Postgres, y la espera por el WAL baja porque casi no hay throttling.

**Carga.** Igual que E4 (`RATE=35000`, 1 minuto, `MAX_VUS=2000`). Las
conexiones son el promedio de 9 fotos (una por scrape) de los últimos 45 s.

**Resultado.**

| | B (1 CPU, pool 10) | E (2 CPU, pool 20) |
|---|---|---|
| Atendido / transacciones de Postgres | 15,2k / 15,2k por s | 28,8k / 28,8k por s |
| CPU de Postgres / throttling | 1,00 / 1,5 s/s | 1,85 / ~0 |
| `idle` (Postgres espera a la app) | 6,0 (60%) | 14,2 (71%) |
| `active` esperando el WAL (`LWLock WALWrite`, `IO WalSync`, `IO WalWrite`) | 3,2 | 4,8 |
| `active` en CPU (sin wait event) | 0,7 | 0,2 |
| `active` en `ClientRead` | 0,1 | 0,8 |
| Tiempo activo que es espera del WAL | 80% | 83% |

- **La mayor parte del tiempo, la conexión retenida está ociosa del lado de
  Postgres.** La app la tiene, pero todavía no mandó la query o está
  procesando la respuesta. Es el tiempo "fuera de la base" que E4 calculaba
  por afuera, ahora visto directamente, y explica por qué agrandar el pool
  rinde poco.
- **Las escrituras son el 10% de los requests y ocupan ~80% del tiempo activo.**
  Cada `INSERT` hace commit y espera a que el WAL llegue a disco
  (`synchronous_commit=on`), y esas escrituras se serializan detrás de
  `WALWriteLock`.
- **pg_stat_activity sirve para ver esperas, no para medir CPU.** En E,
  Postgres usa 1,85 CPU pero casi ninguna foto encuentra una conexión en CPU.
  Las lecturas duran decenas de µs y casi nunca caen en una foto cada 5 s, y
  parte de la CPU es del kernel (red, fsync) y de procesos de fondo. La CPU se
  sigue midiendo con cAdvisor.

**En qué me equivoqué.** Esperaba las conexiones activas y en CPU, y el 60–70%
está ocioso. También supuse que la espera del WAL venía del throttling (el
kernel congelando al proceso que tiene el lock), y no bajó al sacar el
throttling: es el costo propio de cada commit.

## Trampas de medición

Aparecieron en esta fase y van a volver a aparecer en todas:

1. **El generador de carga también se satura.** curl sin keep-alive a ~1.000
   req/s agota los ~28k puertos efímeros del cliente (cada conexión queda 60 s
   en `TIME_WAIT`) y da miles de `000` / `connection reset` que no son culpa
   del sistema. Con keep-alive dieron 40.000 de 40.000 OK. k6 usa keep-alive
   por defecto.
2. **Una corrida no alcanza.** La misma configuración varió ~±7% entre días.
   En una de las corridas, autovacuum se ejecutó en el medio y le sacó CPU a las
   queries.
3. **`increase()` de Prometheus subestima los contadores que nacen en la
   ventana.** El contador de 499 no existía antes de la caída: `increase()` dio
   273, y el valor real del contador era 458. Para totales exactos, restar el
   valor crudo del contador antes y después.
4. **Series de una instancia anterior.** Después de recrear un contenedor,
   Prometheus sigue mostrando hasta 5 minutos las series de la IP vieja (más si
   `make down` también apagó Prometheus y no llegó a marcarlas como vencidas).
   Un `sum` las suma: en E5 aparecieron 30 conexiones con un pool de 20. Filtrar
   por la `instance` actual en vez de esperar.
5. **k6 corre en la misma máquina.** Hay que confirmar que el host no esté al
   tope (`docker stats` o la CPU de todos los contenedores) antes de culpar al
   sistema. En E4, k6 llegó a usar 3,6 CPU.
6. **`docker compose run` recrea las dependencias si cambió su configuración.**
   `make up PG_CPUS=0.5` seguido de un `make load` sin esa variable recreaba
   Postgres en silencio con el valor por defecto, justo antes de la carga. Se
   detectó porque Postgres usaba 1,0 CPU con un límite de 0,5. Por eso `make
   load` usa `--no-deps`. Antes de interpretar una corrida, verificar los
   límites reales (`docker inspect ... NanoCpus`).
7. **La leyenda `mean` de Grafana promedia todo el rango visible.** Incluye los
   minutos sin carga entre corridas: "DB pool: waits/s" mostraba ~13k cuando
   la meseta de la corrida era 25k. Por eso las leyendas del dashboard muestran
   solo `max` y `last`.
8. **`avg_over_time` sobre series intermitentes las infla.** Un wait event que
   aparece en 2 de 9 scrapes se promedia solo sobre esos 2. Las esperas sumaban
   más que las conexiones activas. Para promediar fotos, usar
   `sum_over_time(...) / cantidad de muestras`.

## Conclusiones y mitigaciones

Ninguna está aplicada todavía: son el punto de partida de las fases siguientes.

1. **Timeout de base.** Convierte "colgado indefinidamente" en "error rápido",
   libera la concurrencia y hace visible la falla en las métricas del servidor.
   Implica elegir un valor y decidir qué se le responde al cliente (503/504).
2. **Dimensionar el pool midiendo,** y que pool × réplicas quede bastante por
   debajo de `max_connections`, con margen para admin, monitoreo y
   migraciones. Con muchas réplicas hace falta un pooler (PgBouncer, fase 2).
3. **¿`/readyz` debería depender de la base?** Si depende, una caída de
   Postgres deja a todas las réplicas fuera del load balancer a la vez. Si no,
   el load balancer sigue mandando tráfico a instancias que no pueden
   atenderlo. Es la entrada a la fase 1.
4. **Alertar por saturación, no solo por errores.** Un threshold de
   `dropped_iterations` en k6, o alertas sobre espera en el pool e in-flight,
   detectan lo que el p95 y el % de errores no ven.

## Equivalentes en AWS

| Lab | AWS |
|---|---|
| Postgres | RDS for PostgreSQL / Aurora (el límite de conexiones depende del tamaño de instancia; RDS Proxy cumple el rol de pooler) |
| Prometheus y Grafana | Amazon Managed Service for Prometheus, Amazon Managed Grafana / CloudWatch |
| Loki | CloudWatch Logs |
| cAdvisor | CloudWatch Container Insights |
| postgres_exporter | RDS Performance Insights (carga por wait event) / Enhanced Monitoring |
| k6 | Grafana Cloud k6 / Distributed Load Testing on AWS |

## Qué sigue

**Fase 1: escalado horizontal y load balancing.** Tres réplicas de la app
detrás de un load balancer. Preguntas que deja esta fase:

- ¿Qué hace el load balancer cuando las tres réplicas dan 503 al mismo tiempo
  porque se cayó la base que comparten?
- ¿Escalar la app sube el techo de 15,6k req/s, si el cuello de botella es
  Postgres?
- Tres réplicas con un pool de 10 son 30 conexiones contra `max_connections`
  50: ¿cuántas réplicas entran antes de quedarse sin conexiones?
