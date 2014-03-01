package main

// Installation of required library : 
// GOPATH=`pwd` go get github.com/go-sql-driver/mysql

// Create a database user 'reverse-gol' with password 'reverse-gol' with access rights to database 'reverse-gol'

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

)

/*
CREATE TABLE `problems` ( 
	`id` int(11) NOT NULL, 
	`steps` int(11) NOT NULL, 
	`solution_count` int(11) NOT NULL,
	`currently_processing` int(11) NOT NULL,
	KEY `problems_id` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1
CREATE TABLE `solutions` ( 
	`id` int(11) NOT NULL, 
	`steps` int(11) NOT NULL, 
	`iter` int(11) NOT NULL, 
	`seed` int(11) NOT NULL, 
	`version` int(11) NOT NULL, 
	`mtsi` int(11) DEFAULT NULL, 
	`mtei` int(11) NOT NULL, 
	`mtsf` int(11) DEFAULT NULL, 
	`mtef` int(11) NOT NULL, 
	`start` text NOT NULL, 
	KEY `solutions_id` (`id`) 
) ENGINE=InnoDB DEFAULT CHARSET=latin1
*/

func get_db_connection() *sql.DB {
	//db, err := sql.Open("mysql", "user:password@/database")
	//db, err := sql.Open("mysql", "reverse-gol:reverse-gol@/reverse-gol")
	db, err := sql.Open("mysql", "reverse-gol:reverse-gol@tcp(square.herald:3306)/reverse-gol")
	
	if err != nil {
		panic(err.Error()) // Just for example purpose. You should use proper error handling instead of panic
	}
	
	// Open doesn't open a connection. Validate DSN data:
	err = db.Ping()
	if err != nil {
		panic(err.Error()) // proper error handling instead of panic in your app
	}
	
	return db
}

func test_open_db() {
	db := get_db_connection()
	defer db.Close()
}

func create_list_of_problems_in_db() {
	db := get_db_connection()
	defer db.Close()
	
	//db.Exec("DELETE FROM PROBLEMS")
	
	ins, err := db.Prepare("INSERT INTO problems SET id=?, steps=?, solution_count=?")
	if err != nil {
		fmt.Println("Insert Prepare Error:", err)
		return
	}
	
	for i,f := range []string{"data/test.csv", "data/train.csv", } {
		fmt.Printf("Opening %s - %d\n", f, i)
		
		file, err := os.Open(f)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		defer file.Close()
		
		reader := csv.NewReader(file)
		
		header, err := reader.Read()
		if header[0] != "id" {
			fmt.Println("Bad Header", err)
			return
		}

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Println("Error:", err)
				return
			}

			id, _ := strconv.Atoi(record[0])
			steps, _ := strconv.Atoi(record[1])
			
			if i>0 {
				// This is training data : Let's give it a negative id to avoid confusion
				id = -id
			}
			_, err = ins.Exec(id, steps, 0)
			if err != nil {
				fmt.Println("Insert Error:", err)
				return
			}
		}
	}
	
}

// training examples are stored in the dbs with negative ids.  All ids output from here are corrected to be positive
func list_of_interesting_problems_from_db(steps int, count int, is_training bool) []int {
	db := get_db_connection()
	defer db.Close()
	
	problem_list := []int{}
	
	filter_training_or_test := "id>0"
	if is_training {
		filter_training_or_test = "id<0"
	}
	
	// Only actually need id back
	rows, err := db.Query("SELECT id, steps, solution_count, currently_processing FROM problems"+
							" WHERE currently_processing=0 AND steps=? AND "+filter_training_or_test+
							" ORDER BY solution_count ASC"+ 
							" LIMIT ?", 
							steps, count)
	if err != nil {
		fmt.Println("Query interesting problems Error:", err)
		return problem_list
	}

	update, err := db.Prepare("UPDATE problems SET currently_processing=1 WHERE id=?")
	if err != nil {
		fmt.Println("Update 'currently_processing' Prepare Error:", err)
		return problem_list
	}
	
	for rows.Next() {
		var id, steps, solution_count,currently_processing int
		err = rows.Scan(&id, &steps, &solution_count, &currently_processing)
		//err = rows.Err() // get any error encountered during iteration
		if err != nil {
			fmt.Println("Query interesting problem Error:", err)
			return problem_list
		}
		
		_, err = update.Exec(id)
		if err != nil {
			fmt.Println("Update 'currently_processing' Exec Error:", err)
			return problem_list
		}

		if is_training {
			id=-id
		}
		problem_list = append(problem_list, id)
	}
	
	return problem_list
}

func reset_all_currently_processing(is_training bool) { 
	db := get_db_connection()
	defer db.Close()
	
	filter_training_or_test := "id>0"
	if is_training {
		filter_training_or_test = "id<0"
	}
	
	_, err := db.Exec("UPDATE problems SET currently_processing=0 WHERE "+filter_training_or_test)
	if err != nil {
		fmt.Println("Reset currently_processing Error:", err)
		return 
	}
}

func get_unprocessed_seed_from_db(id int, is_training bool) int {
	db := get_db_connection()
	defer db.Close()
	
	if is_training {
		id = -id
	}

	seed:=1
	rows, err := db.Query("SELECT MAX(seed)+1 FROM solutions WHERE id=?", id)
	if err != nil {
		fmt.Println("Query seed_max Error:", err)
		return seed
	}

	for rows.Next() {
		var seed_max sql.NullInt64
		err = rows.Scan(&seed_max)
		if err != nil {
			fmt.Println("Query seed_max row Error:", err)
			return seed
		}
		if seed_max.Valid {
			seed = int(seed_max.Int64)
		}
	}
	return seed
}


func save_solution_to_db(id int, steps int, seed int, individual_result *IndividualResult, is_training bool) {
	// add to solutions
	db := get_db_connection()
	defer db.Close()
	
	if is_training {
		id = -id // Fix it up
	}
	
	// insert into the solutions db
	_, err := db.Exec("INSERT INTO solutions SET id=?, steps=?, seed=?, version=?, iter=?,"+
						" mtsi=?, mtei=?,"+
						" mtsf=?, mtef=?,"+
						" start=?",
						id, steps, seed,
						currently_running_version, 
						individual_result.iter, 
						individual_result.mismatch_from_true_start_initial, 
						individual_result.mismatch_from_true_end_initial, 
						individual_result.mismatch_from_true_start_final, 
						individual_result.mismatch_from_true_end_final, 
						individual_result.individual.start.toCompactString(),
					)
	if err != nil {
		fmt.Println("Inserting into solutions table for individual Error:", err)
		return
	}
	
	// increment problems solution_count, and reset currently_processing
	_, err = db.Exec("UPDATE problems SET solution_count=solution_count+1, currently_processing=0 WHERE id=?", id)
	if err != nil {
		fmt.Println("Updating problems table for individual Error:", err)
		return 
	}
}

func create_submission(fname string) {
	id_list := []int{}
	
	if true { // true for real submission, false for testing
		for i:=1; i<=50000; i++ {
			id_list = append(id_list, i)
		}
	} else {
		//id_list = append(id_list, -50)
		id_list = append(id_list, -54)
	}
	
	only_submit_for_steps_equals:=-1 // Set this for +ve to filter submission to include only specific steps answers (rest are zeroed as a base-line)
	only_submit_for_steps_equals=5

	only_allow_for_seed_equals  :=-1 // Set this for +ve to filter submission to include only specific seed answers (rest are zeroed as a base-line)
	//only_allow_for_seed_equals  =1
	
	db := get_db_connection()
	defer db.Close()
	
	file, err := os.Create(fname)
	if err != nil {
		fmt.Println("File Creation Error:", err)
		return
	}
	defer file.Close()
	
	file.WriteString("id")
	for i:=1; i<=400; i++ {
		file.WriteString(fmt.Sprintf(",start.%d", i))
	}
	file.WriteString("\n")
	
	query, err := db.Prepare("SELECT steps,iter,seed,version,mtei,mtef,start FROM solutions WHERE id=?")
	if err != nil {
		fmt.Println("Query solutions prepare Error:", err)
		return
	}
	
	count_ids_found, count_zeroes_submitted := 0,0
	for _, id := range id_list {
		rows, err := query.Query(id)
		if err != nil {
			fmt.Println("Query solutions row for id=%d Error:", err)
			return
		}

		type BestRow struct {
			start_board *Board_BoolPacked
			steps,iter,seed,version int
			mtei, mtef int
			valid bool
		}
		best := BestRow{valid:false}
		id_found:=false
		
		submit_zero_for_this_id:=false
		stats := NewBoardStats(board_width, board_height)
		for rows.Next() {
			var start string
			var steps,iter,seed,version int
			var mtei, mtef int
			err = rows.Scan(&steps, &iter, &seed, &version, &mtei, &mtef, &start)
			if err != nil {
				fmt.Println("Query start for id=%d Error:", id, err)
				return 
			}
			id_found = true

			// This zeroes out whole id if condition on one of it's rows fails
			if only_submit_for_steps_equals>0 && steps!=only_submit_for_steps_equals {
				submit_zero_for_this_id=true
				continue
			}
			
			// This ignores this particular row for this id
			// If it's the only row, then we'll end up with a zero for this id
			//   but that won't be included in the count_zeroes_submitted counter
			if version==999 { 
				continue
			}
			
			if only_allow_for_seed_equals>0 && seed!=only_allow_for_seed_equals {
				continue
			}
			if !(seed==1 || seed==2 || seed==3) {
				continue
			}
			
			start_board := NewBoard_BoolPacked(board_width, board_height)
			start_board.fromCompactString(start)
			
			// Do every entry twice ( so that an additional +1 for the best will tie-break a 50/50 threshold)
			start_board.AddToStats(stats)
			start_board.AddToStats(stats)
			//fmt.Println(start_board)
			
			// Figure out whether this is going to be the best board for tie-breaking
			this_is_better_than_current_best := !best.valid // This picks up the first one immediately
			
			// If we ended up with fewer errors, or got to the same level, but quicker, this is probably better
			if mtef<best.mtef || (mtef<best.mtef && iter<best.iter) { 
				this_is_better_than_current_best=true
			}
			
			if this_is_better_than_current_best {
				best = BestRow{start_board, steps, iter, seed, version, mtei, mtef, true}
				fmt.Printf("Best @%5d now %v\n", id, best)
			}
		}
		
		// Now add a single instance of best board to the stats to act as a tie-breaker 
		if best.valid {
			best.start_board.AddToStats(stats)
		}
		if id_found {
			count_ids_found++
		}
		
		//fmt.Println(stats)
		
		// Ok, so now let's figure out a board from these stats that's a better guess
		guess_board := NewBoard_BoolPacked(board_width, board_height)
		//guess_board.ThresholdStats(stats, 50) // This reflects (just the best for n=2), (majority for 3), (majority+boost for 4)
		guess_board.ThresholdStats(stats, 85) // This reflects (all for n=2), (all for n=3), (all for n=4)
		//guess_board.ThresholdStats(stats, 65) // This needs 2/2
		//fmt.Println(guess_board)
		
		// This implements a filter
		if submit_zero_for_this_id {
			// Instead of the true guess, zero it all out
			guess_board = NewBoard_BoolPacked(board_width, board_height)
			count_zeroes_submitted++
		}
		
		file.WriteString(fmt.Sprintf("%d", id))
		file.WriteString(guess_board.toCSV())
		file.WriteString("\n")
	}
	if count_ids_found == len(id_list) {
		if count_ids_found==50000 {
			if count_zeroes_submitted>0 {
				non_zero := count_ids_found-count_zeroes_submitted
				fmt.Printf("FILTERED SUBMISSION FILE :: ONLY HAS %d delta=%d non-zero entries\n", non_zero, only_submit_for_steps_equals) 
			}
			fmt.Printf("TODO : gzip %s\n", fname) 
		} else {
			fmt.Printf("Test file created : %s\n", fname) 
		}
	} else {
		fmt.Printf("BAD SUBMISSION FILE :: ONLY HAS %d of %d IDs!\n", count_ids_found, len(id_list)) 
	}
}


/* :: Useful SQL ::
 * select id,steps,iter from solutions where id>0 and id<50 order by steps,id,iter
 * select steps,seed,count(id) from solutions where id>0 group by steps, seed
 * 
 * How many of each type do we have ::
 * select steps,count(id) from solutions where id>0 group by steps
 * 
 * Diagnose and Fix early aborted runs :: 
 * select steps,count(id) from problems where currently_processing>0 and id>0 group by steps
 * update problems set currently_processing=0 where id>0
 * 
 * What is the state of all solutions ::
 * select steps,solution_count,count(id) from problems where id>0 group by steps, solution_count order by steps,solution_count
 * 
 * Where are there holes to fill ::
 * select steps,solution_count,count(id) from problems where id>0 and solution_count=0 group by steps, solution_count order by steps
 * 
 * What is the distribution of solutions
 * select steps,solution_count,count(id) from problems where id>0 group by steps, solution_count order by steps, solution_count
 * select steps,seed,count(id) from solutions where id>0 group by steps, seed order by steps, seed
 * 
 * What is currently being worked on ::
 * select steps,solution_count,count(id) from problems where id>0 and currently_processing=1 group by steps, solution_count order by steps
 * 
 */